package notify

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"time"
)

// dingTalkChannel implements the DingTalk custom robot.
//
// Platform traits (which drive the choices here):
//   - One bot is limited to 20 messages/minute; excess messages are silently dropped (HTTP
//     may still be 200), so rate limiting must happen on the client, see DefaultRatePerMin.
//   - Security is one of three options: signing / custom keyword / IP allowlist. Signing is
//     the only one independent of message content, so only signing is supported (a bare
//     webhook with none of the three also works).
//   - Success and failure both return HTTP 200; only the errcode in the body distinguishes
//     them -- not checking errcode would record a failed delivery as success.
type dingTalkChannel struct{}

func (dingTalkChannel) Kind() string { return KindDingTalk }

func (dingTalkChannel) DefaultRatePerMin() int { return 20 }

// The DingTalk webhook URL carries access_token, which is itself a credential, so it is
// masked as a whole.
func (dingTalkChannel) SecretKeys() []string { return []string{"webhook", "secret"} }

// The destination is the DingTalk webhook URL itself; changing it must restate the signing
// secret for the new address.
func (dingTalkChannel) DestinationKeys() []string { return []string{"webhook"} }

func (dingTalkChannel) Validate(cfg map[string]any) error {
	hook := cfgString(cfg, "webhook")
	if hook == "" {
		return errors.New("Webhook 주소가 없습니다")
	}
	if err := validateHTTPURL(hook); err != nil {
		return fmt.Errorf("Webhook 주소가 올바르지 않습니다: %w", err)
	}
	return nil
}

// Send delivers one message. A single item with a back-link uses an ActionCard (with a
// button); everything else uses markdown.
func (c dingTalkChannel) Send(ctx context.Context, cfg map[string]any, m Message) (int, error) {
	hook := cfgString(cfg, "webhook")
	if err := c.Validate(cfg); err != nil {
		return 0, Permanent(err)
	}
	endpoint, err := dingTalkSignedURL(hook, cfgString(cfg, "secret"), time.Now())
	if err != nil {
		return 0, Permanent(err)
	}

	title := markdownTitle(m)
	// DingTalk markdown has no explicit byte limit, but a cap is still applied so an
	// abnormally large evidence field cannot bloat the message.
	text, kept := markdownBody(m, 20000)

	var payload any
	if !m.Batch && len(m.Items) == 1 && m.Items[0].DetailURL != "" {
		payload = map[string]any{
			"msgtype": "actionCard",
			"actionCard": map[string]any{
				"title":          title,
				"text":           text,
				"btnOrientation": "0",
				"singleTitle":    "상세 보기",
				"singleURL":      m.Items[0].DetailURL,
			},
		}
	} else {
		payload = map[string]any{
			"msgtype":  "markdown",
			"markdown": map[string]any{"title": title, "text": text},
		}
	}

	raw, err := doJSON(ctx, "POST", endpoint, nil, payload)
	if err != nil {
		return 0, err
	}
	// DingTalk hides business errors inside a 200 response.
	var res struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return 0, fmt.Errorf("DingTalk 응답을 해석하지 못했습니다: %w (%s)", err, snippet(raw))
	}
	if res.ErrCode != 0 {
		// 301000 is a signature check failure, 310000 a keyword mismatch -- both are config
		// errors that a retry will not fix.
		return 0, Permanent(fmt.Errorf("DingTalk에서 오류가 발생했습니다 (%d): %s", res.ErrCode, res.ErrMsg))
	}
	return kept, nil
}

// dingTalkSignedURL appends the timestamp and sign parameters to the webhook URL following
// the official signing rule.
//
// Rule: string to sign = timestamp + "\n" + secret, the HMAC-SHA256 **key is also the
// secret**, and the result is base64 then URL-encoded. timestamp is in milliseconds. An
// empty secret returns the URL unchanged, to support bots without signing.
func dingTalkSignedURL(hook, secret string, now time.Time) (string, error) {
	if secret == "" {
		return hook, nil
	}
	ts := strconv.FormatInt(now.UnixMilli(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts + "\n" + secret))
	sign := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	u, err := url.Parse(hook)
	if err != nil {
		// err is not passed through: url.Parse's error text contains the full address
		// (including access_token).
		return "", fmt.Errorf("Webhook 주소를 해석하지 못했습니다: %s", redactRequestTarget(hook))
	}
	q := u.Query()
	q.Set("timestamp", ts)
	q.Set("sign", sign)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// validateHTTPURL checks that an address is usable and its scheme supported, and applies the
// internal-network check to literal IP targets.
//
// Two points matter:
//
//  1. **The error message must be redacted.** url.Parse itself returns *url.Error, whose
//     Error() carries the **complete original address**, and these providers' addresses embed
//     credentials (DingTalk access_token, WeCom key, Telegram bot token, Feishu hook id). This
//     function once did `return err`, so an "invalid address format" error leaked the
//     credential into the test endpoint's 400 response, every delivery's stored last_error,
//     the server logs and the delivery-history API.
//
//  2. **A literal IP is judged as internal right away**, while hostnames are left to the dial
//     stage (blockInternalDial is the point that finally takes effect and also covers DNS
//     rebinding). Doing it here lets saving the config produce a hint instead of waiting for
//     the first delivery to fail.
//
// Restricting the scheme is defensive: file:///gopher:// make http.Client behave in
// unexpected ways (the scheme check already blocks them, but there is no reason to leave that
// surface open).
func validateHTTPURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("주소를 해석할 수 없습니다 (%s)", redactRequestTarget(raw))
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("http 또는 https 만 지원합니다. 받은 값: %q", u.Scheme)
	}
	if u.Host == "" {
		return errors.New("호스트 이름이 없습니다")
	}
	if ip := net.ParseIP(u.Hostname()); ip != nil && isBlockedDialIP(ip) && !allowLocalTargets() {
		return fmt.Errorf("로컬 또는 링크 로컬 주소 %s 로는 전송하지 않습니다 (로컬 서비스로 보내야 하면 %s=1 로 설정하세요)", ip, AllowLocalTargetsEnv)
	}
	return nil
}
