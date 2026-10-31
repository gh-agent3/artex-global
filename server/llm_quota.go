package server

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Per-profile subscription quota (Kimi Code /usages and xAI SuperGrok billing).
// The LLM card shows the provider's own limit windows (5h, weekly, monthly) —
// not ARTEX's rate limits. Fetched lazily with a short TTL so the LLM page never
// hammers the provider on every render; the API key never leaves this process.

type llmQuotaWindow struct {
	Key            string  `json:"key"` // 5h | week | month_total | month_code
	UsedRatio      float64 `json:"used_ratio"`
	RemainingRatio float64 `json:"remaining_ratio"`
	ResetTime      string  `json:"reset_time,omitempty"`
}

type llmQuotaReply struct {
	Supported bool             `json:"supported"`
	Quotas    []llmQuotaWindow `json:"quotas,omitempty"`
	FetchedAt string           `json:"fetched_at,omitempty"`
	Error     string           `json:"error,omitempty"`
}

type llmQuotaCacheEntry struct {
	reply llmQuotaReply
	at    time.Time
}

var llmQuotaCache = struct {
	sync.Mutex
	m map[int64]llmQuotaCacheEntry
}{m: map[int64]llmQuotaCacheEntry{}}

const (
	llmQuotaTTL      = 60 * time.Second
	llmQuotaErrTTL   = 20 * time.Second
	llmQuotaHTTPWait = 10 * time.Second
)

// kimiUsagesURL resolves the /usages endpoint for a Kimi Code profile. Returns
// "" for every other provider.
func kimiUsagesURL(baseURL string) string {
	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	if host != "api.kimi.com" && host != "api.kimi.ai" {
		return ""
	}
	path := strings.TrimRight(u.Path, "/")
	if !strings.HasSuffix(path, "/coding") {
		return ""
	}
	return u.Scheme + "://" + u.Host + path + "/v1/usages"
}

// grokQuotaURL resolves the billing quota endpoint for a SuperGrok profile
// (either through the modelrouter think listener's /v1/grok/quota route or
// directly against cli-chat-proxy.grok.com).
func grokQuotaURL(baseURL, model string) (endpoint string, directXAI bool) {
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(model)), "grok") {
		return "", false
	}
	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", false
	}
	host := strings.ToLower(u.Hostname())
	if host == "cli-chat-proxy.grok.com" || host == "api.x.ai" {
		return "https://cli-chat-proxy.grok.com/v1/billing?format=credits", true
	}
	path := strings.TrimRight(u.Path, "/")
	if !strings.HasSuffix(path, "/v1") {
		path += "/v1"
	}
	return u.Scheme + "://" + u.Host + path + "/grok/quota", false
}

func quotaSupported(baseURL, model string) bool {
	if kimiUsagesURL(baseURL) != "" {
		return true
	}
	ep, _ := grokQuotaURL(baseURL, model)
	return ep != ""
}

func fetchKimiQuota(baseURL, apiKey string) (llmQuotaReply, error) {
	endpoint := kimiUsagesURL(baseURL)
	if endpoint == "" {
		return llmQuotaReply{Supported: false}, nil
	}
	if apiKey == "" {
		return llmQuotaReply{}, fmt.Errorf("no API key on the profile")
	}
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return llmQuotaReply{}, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	client := &http.Client{Timeout: llmQuotaHTTPWait}
	resp, err := client.Do(req)
	if err != nil {
		return llmQuotaReply{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return llmQuotaReply{}, fmt.Errorf("provider returned HTTP %d", resp.StatusCode)
	}
	var payload struct {
		Usages map[string]struct {
			UsedRatio float64 `json:"used_ratio"`
			ResetTime string  `json:"reset_time"`
		} `json:"usages"`
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return llmQuotaReply{}, err
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return llmQuotaReply{}, err
	}
	reply := llmQuotaReply{Supported: true, FetchedAt: time.Now().UTC().Format(time.RFC3339)}
	for _, key := range []string{"limit_5h", "limit_month_total", "limit_month_code"} {
		u, ok := payload.Usages[key]
		if !ok {
			continue
		}
		reply.Quotas = append(reply.Quotas, llmQuotaWindow{
			Key:            strings.TrimPrefix(key, "limit_"),
			UsedRatio:      u.UsedRatio,
			RemainingRatio: 1 - u.UsedRatio,
			ResetTime:      u.ResetTime,
		})
	}
	if len(reply.Quotas) == 0 {
		return llmQuotaReply{}, fmt.Errorf("no usage windows in the provider response")
	}
	return reply, nil
}

func parseGrokQuotaBody(body []byte) (llmQuotaReply, error) {
	var payload struct {
		Config *struct {
			CurrentPeriod *struct {
				Type  string `json:"type"`
				Start string `json:"start"`
				End   string `json:"end"`
			} `json:"currentPeriod"`
			CreditUsagePercent *float64 `json:"creditUsagePercent"`
			BillingPeriodEnd   string   `json:"billingPeriodEnd"`
		} `json:"config"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return llmQuotaReply{}, err
	}
	if payload.Config == nil {
		return llmQuotaReply{}, fmt.Errorf("no billing config in the provider response")
	}
	// Protobuf JSON omits zero-valued fields, so an absent creditUsagePercent with
	// a present currentPeriod means 0% used rather than missing quota.
	usedPct := 0.0
	if payload.Config.CreditUsagePercent != nil {
		usedPct = *payload.Config.CreditUsagePercent
	} else if payload.Config.CurrentPeriod == nil {
		return llmQuotaReply{}, fmt.Errorf("no usage window in the provider response")
	}
	usedRatio := math.Max(0, math.Min(1, usedPct/100.0))
	key := "week"
	resetTime := payload.Config.BillingPeriodEnd
	if p := payload.Config.CurrentPeriod; p != nil {
		upper := strings.ToUpper(p.Type)
		if strings.Contains(upper, "MONTH") {
			key = "month_total"
		}
		if p.End != "" {
			resetTime = p.End
		}
	}
	return llmQuotaReply{
		Supported: true,
		FetchedAt: time.Now().UTC().Format(time.RFC3339),
		Quotas: []llmQuotaWindow{{
			Key:            key,
			UsedRatio:      usedRatio,
			RemainingRatio: 1 - usedRatio,
			ResetTime:      resetTime,
		}},
	}, nil
}

func fetchGrokQuota(baseURL, model, apiKey string) (llmQuotaReply, error) {
	endpoint, directXAI := grokQuotaURL(baseURL, model)
	if endpoint == "" {
		return llmQuotaReply{Supported: false}, nil
	}
	if apiKey == "" {
		return llmQuotaReply{}, fmt.Errorf("no API key on the profile")
	}
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return llmQuotaReply{}, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")
	if directXAI {
		req.Header.Set("User-Agent", "OpenCode-Quota-Toast/1.0")
		req.Header.Set("x-grok-client-surface", "grok-build")
		req.Header.Set("x-grok-client-version", "1.0.0")
	}
	client := &http.Client{Timeout: llmQuotaHTTPWait}
	resp, err := client.Do(req)
	if err != nil {
		return llmQuotaReply{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return llmQuotaReply{}, fmt.Errorf("provider returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return llmQuotaReply{}, err
	}
	return parseGrokQuotaBody(body)
}

func fetchProfileQuota(baseURL, model, apiKey string) (llmQuotaReply, error) {
	if kimiUsagesURL(baseURL) != "" {
		return fetchKimiQuota(baseURL, apiKey)
	}
	return fetchGrokQuota(baseURL, model, apiKey)
}

func (s *Server) pgProfileQuota(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	id, _ := pathInt(r, "id")
	p, err := pg.ProfileByID(id)
	if err != nil || p == nil {
		writeErr(w, 404, "profile not found")
		return
	}
	if !quotaSupported(p.BaseURL, p.Model) {
		writeJSON(w, 200, llmQuotaReply{Supported: false})
		return
	}
	llmQuotaCache.Lock()
	entry, ok := llmQuotaCache.m[id]
	llmQuotaCache.Unlock()
	ttl := llmQuotaTTL
	if ok && entry.reply.Error != "" {
		ttl = llmQuotaErrTTL
	}
	if ok && time.Since(entry.at) < ttl {
		writeJSON(w, 200, entry.reply)
		return
	}
	reply, err := fetchProfileQuota(p.BaseURL, p.Model, p.APIKey)
	if err != nil {
		reply = llmQuotaReply{Supported: true, Error: err.Error()}
	}
	llmQuotaCache.Lock()
	llmQuotaCache.m[id] = llmQuotaCacheEntry{reply: reply, at: time.Now()}
	llmQuotaCache.Unlock()
	writeJSON(w, 200, reply)
}
