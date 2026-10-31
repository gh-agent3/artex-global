package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"text/template"
	"time"
)

// webhookChannel is the generic webhook adapter: user-defined URL, method, headers and JSON
// template. It means this feature does not need a separate implementation for Slack /
// Mattermost / Discord / in-house systems -- all of them can be covered by a configurable
// template.
type webhookChannel struct{}

func (webhookChannel) Kind() string { return KindWebhook }

// A generic webhook has no official limit; 0 means no rate limit by default, left to the user
// based on the peer's capacity.
func (webhookChannel) DefaultRatePerMin() int { return 0 }

// Mask url and headers: the destination address itself often carries a token, and custom
// headers usually hold auth credentials; both appear in API responses, so both must be
// blocked.
// The cost is that editing one header requires re-entering the whole header set (a masked
// value means "keep the stored one") -- a deliberate tradeoff: fill it in once more rather
// than echo credentials to the browser.
func (webhookChannel) SecretKeys() []string { return []string{"url", "headers"} }

// The destination is url. Changing url requires restating headers -- otherwise the original
// Authorization header would be sent verbatim to the new address, which is the main
// mask-bypass path.
func (webhookChannel) DestinationKeys() []string { return []string{"url"} }

// webhookDefaultTemplate is the fallback request body when no template is configured: a
// straightforward JSON structure covering most "store one JSON" in-house receivers.
const webhookDefaultTemplate = `{
  "title": {{json .Title}},
  "batch": {{.Batch}},
  "count": {{.Count}},
  "items": [
{{- range $i, $it := .Items}}
{{- if $i}},{{end}}
    {
      "finding_id": {{$it.FindingID}},
      "name": {{json $it.Name}},
      "vulnclass": {{json $it.VulnClass}},
      "severity": {{json $it.Severity}},
      "summary": {{json $it.Summary}},
      "assets": {{json $it.Assets}},
      "detail_url": {{json $it.DetailURL}}
    }
{{- end}}
  ]
}`

// webhookTemplateData is the context exposed to user templates.
type webhookTemplateData struct {
	Title   string
	Batch   bool
	Count   int
	Items   []webhookItem
	HomeURL string
	// SentAt is the delivery time (RFC3339), for the receiver to record.
	SentAt string
}

type webhookItem struct {
	FindingID     int64
	Name          string
	VulnClass     string
	Severity      string
	SeverityLabel string
	Summary       string
	Assets        []string
	DetailURL     string
	FromStatus    string
	ToStatus      string
	// StatusLabel is the human-readable status transition, e.g. "pending -> fixed"; empty when
	// this is not a status change.
	StatusLabel string
}

func (webhookChannel) Validate(cfg map[string]any) error {
	raw := cfgString(cfg, "url")
	if raw == "" {
		return errors.New("대상 URL이 없습니다")
	}
	if err := validateHTTPURL(raw); err != nil {
		return fmt.Errorf("대상 URL이 올바르지 않습니다: %w", err)
	}
	if m := strings.ToUpper(cfgString(cfg, "method")); m != "" && m != http.MethodGet && m != http.MethodPost && m != http.MethodPut && m != http.MethodPatch {
		return fmt.Errorf("지원하지 않는 메서드입니다: %s (GET/POST/PUT/PATCH 중에서 선택하세요)", m)
	}
	if tpl := cfgString(cfg, "body_template"); tpl != "" {
		if _, err := parseWebhookTemplate(tpl); err != nil {
			return fmt.Errorf("요청 본문 템플릿 문법이 올바르지 않습니다: %w", err)
		}
	}
	return nil
}

func (c webhookChannel) Send(ctx context.Context, cfg map[string]any, m Message) (int, error) {
	if err := c.Validate(cfg); err != nil {
		return 0, Permanent(err)
	}
	method := strings.ToUpper(cfgString(cfg, "method"))
	if method == "" {
		method = http.MethodPost
	}

	// GET carries no body: putting content into the query is beyond what the template can
	// express and does not match GET semantics, so GET only suits "hit and trigger" receivers.
	var payload any
	if method != http.MethodGet {
		body, err := renderWebhookBody(cfgString(cfg, "body_template"), m)
		if err != nil {
			return 0, Permanent(err)
		}
		// The template renders string-shaped JSON; it is turned into json.RawMessage and sent
		// as is, so double escaping cannot wrap the user's carefully built structure in a JSON
		// string.
		if !json.Valid([]byte(body)) {
			return 0, Permanent(errors.New("요청 본문 템플릿의 렌더링 결과가 올바른 JSON이 아닙니다"))
		}
		payload = json.RawMessage(body)
	}

	headers := cfgMap(cfg, "headers")
	if ct := cfgString(cfg, "content_type"); ct != "" {
		// Overriding is allowed, but applied after headers so an explicit setting wins.
		if headers == nil {
			headers = map[string]string{}
		}
		headers["Content-Type"] = ct
	}
	if _, err := doJSON(ctx, method, cfgString(cfg, "url"), headers, payload); err != nil {
		return 0, err
	}
	// A generic webhook body is not truncated (the receiver is the user's own service and the
	// size is decided by body_template), so the whole batch counts as delivered.
	return len(m.Items), nil
}

// renderWebhookBody renders the request body with the user template (or the default one).
func renderWebhookBody(tpl string, m Message) (string, error) {
	if strings.TrimSpace(tpl) == "" {
		tpl = webhookDefaultTemplate
	}
	t, err := parseWebhookTemplate(tpl)
	if err != nil {
		return "", fmt.Errorf("요청 본문 템플릿 문법이 올바르지 않습니다: %w", err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, newWebhookTemplateData(m)); err != nil {
		return "", fmt.Errorf("요청 본문 템플릿을 렌더링하지 못했습니다: %w", err)
	}
	return buf.String(), nil
}

// parseWebhookTemplate parses the template.
//
// missingkey=zero renders a missing map key as a zero value instead of erroring -- but the
// context here is a struct, so its main effect is keeping range over an empty .Items working.
// What really needs guarding is .Items being nil.
func parseWebhookTemplate(tpl string) (*template.Template, error) {
	return template.New("body").Funcs(webhookTemplateFuncs).Option("missingkey=zero").Parse(tpl)
}

// webhookTemplateFuncs are the helper functions exposed to templates.
var webhookTemplateFuncs = template.FuncMap{
	// json serializes any value to JSON.
	//
	// This function is not a nicety but a necessity: without it the user could only write
	// {{.Title}} directly, and a single quote or newline in a finding title would make the
	// whole body invalid JSON -- the receiver rejects it and the error points at "JSON parse
	// failure", giving no hint that the title contains a quote.
	"json": func(v any) (string, error) {
		raw, err := json.Marshal(v)
		if err != nil {
			return "", err
		}
		return string(raw), nil
	},
	// jsons embeds a JSON fragment inside another JSON string value (one level of string
	// escaping).
	"jsons": func(v any) (string, error) {
		raw, err := json.Marshal(v)
		if err != nil {
			return "", err
		}
		quoted, err := json.Marshal(string(raw))
		if err != nil {
			return "", err
		}
		// Strip the outer quotes: the caller decides whether to add them.
		return string(quoted[1 : len(quoted)-1]), nil
	},
}

func newWebhookTemplateData(m Message) webhookTemplateData {
	d := webhookTemplateData{
		Title:   markdownTitle(m),
		Batch:   m.Batch,
		Count:   len(m.Items),
		HomeURL: m.HomeURL,
		SentAt:  time.Now().Format(time.RFC3339),
		Items:   make([]webhookItem, 0, len(m.Items)),
	}
	for _, it := range m.Items {
		wi := webhookItem{
			FindingID:     it.FindingID,
			Name:          it.Name,
			VulnClass:     it.VulnClass,
			Severity:      it.Severity,
			SeverityLabel: SeverityLabel(it.Severity),
			Summary:       it.Summary,
			Assets:        append([]string{}, it.Assets...),
			DetailURL:     it.DetailURL,
			FromStatus:    it.FromStatus,
			ToStatus:      it.ToStatus,
		}
		if it.IsStatusChange() {
			wi.StatusLabel = StatusLabel(it.FromStatus) + " → " + StatusLabel(it.ToStatus)
		}
		d.Items = append(d.Items, wi)
	}
	return d
}
