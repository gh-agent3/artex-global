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

	"github.com/Autumn-27/artex/db"
)

// Per-profile usage for the LLM card: the provider's own subscription windows
// and account balance (Kimi Code, xAI SuperGrok, ChatGPT/Codex, DeepSeek) plus
// what ARTEX itself metered in llm_usage. Provider calls are cached for a short
// TTL so the LLM page never hammers a provider on every render; the API key
// never leaves this process.

type llmQuotaWindow struct {
	Key            string  `json:"key"` // 5h | week | month_total | month_code | code_review
	UsedRatio      float64 `json:"used_ratio"`
	RemainingRatio float64 `json:"remaining_ratio"`
	Used           string  `json:"used,omitempty"`
	Limit          string  `json:"limit,omitempty"`
	Remaining      string  `json:"remaining,omitempty"`
	ResetTime      string  `json:"reset_time,omitempty"`
	StartTime      string  `json:"start_time,omitempty"`
}

type llmBalanceItem struct {
	Currency string `json:"currency"` // USD | CNY
	Total    string `json:"total"`
	Granted  string `json:"granted,omitempty"`
	ToppedUp string `json:"topped_up,omitempty"`
}

type llmQuotaReply struct {
	Supported bool             `json:"supported"`
	Plan      string           `json:"plan,omitempty"`
	Account   string           `json:"account,omitempty"`
	RenewsAt  string           `json:"renews_at,omitempty"`
	Quotas    []llmQuotaWindow `json:"quotas,omitempty"`
	Balances  []llmBalanceItem `json:"balances,omitempty"`
	Usage     *llmUsageStats   `json:"usage,omitempty"`
	FetchedAt string           `json:"fetched_at,omitempty"`
	Error     string           `json:"error,omitempty"`
}

// llmUsageStats mirrors db.ProfileUsageStats on the wire.
type llmUsageStats struct {
	Calls            int    `json:"calls"`
	Errors           int    `json:"errors"`
	InputTokens      int    `json:"input_tokens"`
	OutputTokens     int    `json:"output_tokens"`
	CacheReadTokens  int    `json:"cache_read_tokens"`
	CacheWriteTokens int    `json:"cache_write_tokens"`
	TodayTokens      int    `json:"today_tokens"`
	LastUsedAt       string `json:"last_used_at,omitempty"`
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

func kimiMeURL(baseURL string) string {
	usages := kimiUsagesURL(baseURL)
	if usages == "" {
		return ""
	}
	return strings.TrimSuffix(usages, "/usages") + "/me"
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

// chatgptQuotaURL resolves the Codex quota endpoint: ChatGPT's own wham/usage
// for a direct chatgpt.com profile, otherwise the modelrouter chatgpt-proxy's
// /v1/quota route.
func chatgptQuotaURL(baseURL, model string) string {
	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	if host == "chatgpt.com" {
		return "https://chatgpt.com/backend-api/wham/usage"
	}
	m := strings.ToLower(strings.TrimSpace(model))
	isGPT := strings.HasPrefix(m, "gpt-") || strings.Contains(m, "-sol") ||
		strings.Contains(m, "-astra") || strings.Contains(m, "-luna") || strings.Contains(m, "-terra")
	if u.Port() != "4016" && !isGPT {
		return ""
	}
	path := strings.TrimRight(u.Path, "/")
	path = strings.TrimSuffix(path, "/responses")
	if !strings.HasSuffix(path, "/v1") {
		path += "/v1"
	}
	return u.Scheme + "://" + u.Host + path + "/quota"
}

// deepseekQuotaURL resolves the prepaid balance endpoint: DeepSeek's own
// /user/balance for a direct api.deepseek.com profile, otherwise the
// modelrouter think listener's /v1/deepseek/quota route.
func deepseekQuotaURL(baseURL, model string) string {
	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	if host == "api.deepseek.com" {
		return "https://api.deepseek.com/user/balance"
	}
	m := strings.ToLower(strings.TrimSpace(model))
	if (host == "172.17.0.1" || host == "127.0.0.1" || host == "localhost") && u.Port() == "4012" &&
		(m == "think" || strings.HasPrefix(m, "deepseek")) {
		path := strings.TrimRight(u.Path, "/")
		if !strings.HasSuffix(path, "/v1") {
			path += "/v1"
		}
		return u.Scheme + "://" + u.Host + path + "/deepseek/quota"
	}
	return ""
}

func quotaSupported(baseURL, model string) bool {
	if kimiUsagesURL(baseURL) != "" {
		return true
	}
	if ep, _ := grokQuotaURL(baseURL, model); ep != "" {
		return true
	}
	if chatgptQuotaURL(baseURL, model) != "" {
		return true
	}
	return deepseekQuotaURL(baseURL, model) != ""
}

// getJSON performs one provider GET with a bearer key and decodes the body.
func getJSON(endpoint, apiKey string, headers map[string]string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	client := &http.Client{Timeout: llmQuotaHTTPWait}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("provider returned HTTP %d", resp.StatusCode)
	}
	return body, nil
}

func fetchKimiQuota(baseURL, apiKey string) (llmQuotaReply, error) {
	endpoint := kimiUsagesURL(baseURL)
	if endpoint == "" {
		return llmQuotaReply{Supported: false}, nil
	}
	if apiKey == "" {
		return llmQuotaReply{}, fmt.Errorf("no API key on the profile")
	}
	body, err := getJSON(endpoint, apiKey, nil)
	if err != nil {
		return llmQuotaReply{}, err
	}
	var payload struct {
		Limits []struct {
			Detail struct {
				Limit     string `json:"limit"`
				Used      string `json:"used"`
				Remaining string `json:"remaining"`
				ResetTime string `json:"resetTime"`
			} `json:"detail"`
		} `json:"limits"`
		Usages map[string]struct {
			UsedRatio float64 `json:"used_ratio"`
			ResetTime string  `json:"reset_time"`
		} `json:"usages"`
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
		w := llmQuotaWindow{
			Key:            strings.TrimPrefix(key, "limit_"),
			UsedRatio:      u.UsedRatio,
			RemainingRatio: math.Max(0, 1-u.UsedRatio),
			ResetTime:      u.ResetTime,
		}
		if w.Key == "5h" && len(payload.Limits) > 0 {
			d := payload.Limits[0].Detail
			w.Used, w.Limit, w.Remaining = d.Used, d.Limit, d.Remaining
			if w.ResetTime == "" && d.ResetTime != "" {
				w.ResetTime = d.ResetTime
			}
		}
		reply.Quotas = append(reply.Quotas, w)
	}
	if len(reply.Quotas) == 0 {
		return llmQuotaReply{}, fmt.Errorf("no usage windows in the provider response")
	}
	// Kimi's /me carries the plan tier and nickname. It uses the anthropic
	// headers, and a failure here must not lose the quota windows above.
	if meURL := kimiMeURL(baseURL); meURL != "" {
		req, err := http.NewRequest(http.MethodGet, meURL, nil)
		if err == nil {
			req.Header.Set("x-api-key", apiKey)
			req.Header.Set("anthropic-version", "2023-06-01")
			if resp, err := (&http.Client{Timeout: 3 * time.Second}).Do(req); err == nil {
				defer resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					var me struct {
						Nickname      string `json:"nickname"`
						UserLevelName string `json:"user_level_name"`
					}
					if b, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10)); err == nil && json.Unmarshal(b, &me) == nil {
						reply.Account = me.Nickname
						if me.UserLevelName != "" {
							reply.Plan = "Kimi " + me.UserLevelName
						}
					}
				}
			}
		}
	}
	return reply, nil
}

func formatGrokPlan(plan, tier string) string {
	switch strings.ToUpper(tier) {
	case "SUBSCRIPTION_TIER_SUPER_GROK_HEAVY":
		return "SuperGrok Heavy"
	case "SUBSCRIPTION_TIER_SUPER_GROK_PLUS", "SUBSCRIPTION_TIER_SUPER_GROK_PRO":
		return "SuperGrok Plus"
	case "SUBSCRIPTION_TIER_SUPER_GROK_LITE":
		return "SuperGrok Lite"
	}
	if plan != "" {
		return plan
	}
	return "SuperGrok"
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
			PrepaidBalance     *struct {
				Val float64 `json:"val"`
			} `json:"prepaidBalance"`
		} `json:"config"`
		Subscription *struct {
			Plan             string `json:"plan"`
			Tier             string `json:"tier"`
			BillingPeriodEnd string `json:"billingPeriodEnd"`
		} `json:"subscription"`
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
	startTime := ""
	if p := payload.Config.CurrentPeriod; p != nil {
		if strings.Contains(strings.ToUpper(p.Type), "MONTH") {
			key = "month_total"
		}
		if p.End != "" {
			resetTime = p.End
		}
		startTime = p.Start
	}
	reply := llmQuotaReply{
		Supported: true,
		Plan:      "SuperGrok",
		FetchedAt: time.Now().UTC().Format(time.RFC3339),
		Quotas: []llmQuotaWindow{{
			Key:            key,
			UsedRatio:      usedRatio,
			RemainingRatio: 1 - usedRatio,
			Used:           fmt.Sprintf("%.0f%%", usedPct),
			Limit:          "100%",
			ResetTime:      resetTime,
			StartTime:      startTime,
		}},
	}
	if sub := payload.Subscription; sub != nil {
		reply.Plan = formatGrokPlan(sub.Plan, sub.Tier)
		reply.RenewsAt = sub.BillingPeriodEnd
	}
	if pb := payload.Config.PrepaidBalance; pb != nil && pb.Val > 0 {
		reply.Balances = append(reply.Balances, llmBalanceItem{
			Currency: "USD",
			Total:    fmt.Sprintf("%.2f", pb.Val/100.0),
		})
	}
	return reply, nil
}

func fetchGrokQuota(baseURL, model, apiKey string) (llmQuotaReply, error) {
	endpoint, directXAI := grokQuotaURL(baseURL, model)
	if endpoint == "" {
		return llmQuotaReply{Supported: false}, nil
	}
	if apiKey == "" {
		return llmQuotaReply{}, fmt.Errorf("no API key on the profile")
	}
	headers := map[string]string{}
	if directXAI {
		headers["User-Agent"] = "OpenCode-Quota-Toast/1.0"
		headers["x-grok-client-surface"] = "grok-build"
		headers["x-grok-client-version"] = "1.0.0"
	}
	body, err := getJSON(endpoint, apiKey, headers)
	if err != nil {
		return llmQuotaReply{}, err
	}
	return parseGrokQuotaBody(body)
}

type chatgptWindowRaw struct {
	UsedPercent        *float64 `json:"used_percent"`
	RemainingPercent   *float64 `json:"remaining_percent"`
	LimitWindowSeconds int64    `json:"limit_window_seconds"`
	ResetAfterSeconds  int64    `json:"reset_after_seconds"`
	ResetAt            int64    `json:"reset_at"`
}

func parseChatGPTWindow(w *chatgptWindowRaw, fallbackKey string) (llmQuotaWindow, bool) {
	if w == nil {
		return llmQuotaWindow{}, false
	}
	var usedRatio float64
	switch {
	case w.UsedPercent != nil:
		usedRatio = math.Max(0, math.Min(1, *w.UsedPercent/100.0))
	case w.RemainingPercent != nil:
		usedRatio = math.Max(0, math.Min(1, 1-*w.RemainingPercent/100.0))
	default:
		return llmQuotaWindow{}, false
	}
	key := fallbackKey
	switch w.LimitWindowSeconds {
	case 18000:
		key = "5h"
	case 604800:
		key = "week"
	case 2628000:
		key = "month_total"
	}
	resetTime, startTime := "", ""
	if w.ResetAt > 0 {
		t := time.Unix(w.ResetAt, 0).UTC()
		resetTime = t.Format(time.RFC3339)
		if w.LimitWindowSeconds > 0 {
			startTime = t.Add(-time.Duration(w.LimitWindowSeconds) * time.Second).Format(time.RFC3339)
		}
	} else if w.ResetAfterSeconds > 0 {
		resetTime = time.Now().UTC().Add(time.Duration(w.ResetAfterSeconds) * time.Second).Format(time.RFC3339)
	}
	return llmQuotaWindow{
		Key:            key,
		UsedRatio:      usedRatio,
		RemainingRatio: 1 - usedRatio,
		Used:           fmt.Sprintf("%.0f%%", usedRatio*100),
		Limit:          "100%",
		ResetTime:      resetTime,
		StartTime:      startTime,
	}, true
}

func parseChatGPTQuotaBody(body []byte) (llmQuotaReply, error) {
	var payload struct {
		Email     string `json:"email"`
		PlanType  string `json:"plan_type"`
		RateLimit *struct {
			PrimaryWindow   *chatgptWindowRaw `json:"primary_window"`
			SecondaryWindow *chatgptWindowRaw `json:"secondary_window"`
		} `json:"rate_limit"`
		CodeReviewRateLimit *struct {
			PrimaryWindow *chatgptWindowRaw `json:"primary_window"`
		} `json:"code_review_rate_limit"`
		SpendControl *struct {
			IndividualLimit *chatgptWindowRaw `json:"individual_limit"`
		} `json:"spend_control"`
		Credits *struct {
			HasCredits bool   `json:"has_credits"`
			Balance    string `json:"balance"`
		} `json:"credits"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return llmQuotaReply{}, err
	}
	plan := "GPT"
	if p := strings.TrimSpace(payload.PlanType); p != "" {
		plan = "GPT (" + p + ")"
	}
	reply := llmQuotaReply{
		Supported: true,
		Plan:      plan,
		Account:   payload.Email,
		FetchedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if payload.RateLimit != nil {
		if w, ok := parseChatGPTWindow(payload.RateLimit.PrimaryWindow, "week"); ok {
			reply.Quotas = append(reply.Quotas, w)
		}
		if w, ok := parseChatGPTWindow(payload.RateLimit.SecondaryWindow, "5h"); ok {
			reply.Quotas = append(reply.Quotas, w)
		}
	}
	if payload.SpendControl != nil {
		if w, ok := parseChatGPTWindow(payload.SpendControl.IndividualLimit, "month_total"); ok {
			reply.Quotas = append(reply.Quotas, w)
		}
	}
	if payload.CodeReviewRateLimit != nil {
		if w, ok := parseChatGPTWindow(payload.CodeReviewRateLimit.PrimaryWindow, "code_review"); ok {
			w.Key = "code_review"
			reply.Quotas = append(reply.Quotas, w)
		}
	}
	if payload.Credits != nil && payload.Credits.HasCredits &&
		payload.Credits.Balance != "" && payload.Credits.Balance != "0" {
		reply.Balances = append(reply.Balances, llmBalanceItem{
			Currency: "USD",
			Total:    payload.Credits.Balance,
		})
	}
	if len(reply.Quotas) == 0 && len(reply.Balances) == 0 {
		return llmQuotaReply{}, fmt.Errorf("no quota windows in the provider response")
	}
	return reply, nil
}

func fetchChatGPTQuota(baseURL, model, apiKey string) (llmQuotaReply, error) {
	endpoint := chatgptQuotaURL(baseURL, model)
	if endpoint == "" {
		return llmQuotaReply{Supported: false}, nil
	}
	if apiKey == "" {
		return llmQuotaReply{}, fmt.Errorf("no API key on the profile")
	}
	body, err := getJSON(endpoint, apiKey, map[string]string{
		"User-Agent": "OpenCode-Quota-Toast/1.0",
	})
	if err != nil {
		return llmQuotaReply{}, err
	}
	return parseChatGPTQuotaBody(body)
}

func parseDeepSeekQuotaBody(body []byte) (llmQuotaReply, error) {
	var payload struct {
		IsAvailable  *bool `json:"is_available"`
		BalanceInfos []struct {
			Currency        string `json:"currency"`
			TotalBalance    string `json:"total_balance"`
			GrantedBalance  string `json:"granted_balance"`
			ToppedUpBalance string `json:"topped_up_balance"`
		} `json:"balance_infos"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return llmQuotaReply{}, err
	}
	reply := llmQuotaReply{
		Supported: true,
		Plan:      "DeepSeek API",
		FetchedAt: time.Now().UTC().Format(time.RFC3339),
	}
	for _, b := range payload.BalanceInfos {
		if b.TotalBalance == "" {
			continue
		}
		reply.Balances = append(reply.Balances, llmBalanceItem{
			Currency: strings.ToUpper(b.Currency),
			Total:    b.TotalBalance,
			Granted:  b.GrantedBalance,
			ToppedUp: b.ToppedUpBalance,
		})
	}
	if len(reply.Balances) == 0 {
		return llmQuotaReply{}, fmt.Errorf("no balance entries in the provider response")
	}
	return reply, nil
}

func fetchDeepSeekQuota(baseURL, model, apiKey string) (llmQuotaReply, error) {
	endpoint := deepseekQuotaURL(baseURL, model)
	if endpoint == "" {
		return llmQuotaReply{Supported: false}, nil
	}
	if apiKey == "" {
		return llmQuotaReply{}, fmt.Errorf("no API key on the profile")
	}
	body, err := getJSON(endpoint, apiKey, map[string]string{
		"User-Agent": "OpenCode-Quota-Toast/1.0",
	})
	if err != nil {
		return llmQuotaReply{}, err
	}
	return parseDeepSeekQuotaBody(body)
}

func fetchProfileQuota(baseURL, model, apiKey string) (llmQuotaReply, error) {
	if kimiUsagesURL(baseURL) != "" {
		return fetchKimiQuota(baseURL, apiKey)
	}
	if endpoint, _ := grokQuotaURL(baseURL, model); endpoint != "" {
		return fetchGrokQuota(baseURL, model, apiKey)
	}
	if chatgptQuotaURL(baseURL, model) != "" {
		return fetchChatGPTQuota(baseURL, model, apiKey)
	}
	return fetchDeepSeekQuota(baseURL, model, apiKey)
}

func usageStatsWire(s db.ProfileUsageStats) *llmUsageStats {
	return &llmUsageStats{
		Calls:            s.Calls,
		Errors:           s.Errors,
		InputTokens:      s.InputTokens,
		OutputTokens:     s.OutputTokens,
		CacheReadTokens:  s.CacheReadTokens,
		CacheWriteTokens: s.CacheWriteTokens,
		TodayTokens:      s.TodayTokens,
		LastUsedAt:       s.LastUsedAt,
	}
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
	supported := quotaSupported(p.BaseURL, p.Model)
	if !supported {
		// Even with no provider quota endpoint the card still shows what ARTEX
		// metered for this profile, so the page is never blank.
		reply := llmQuotaReply{Supported: false}
		if stats, err := pg.ProfileUsageStats(p.Name); err == nil {
			reply.Usage = usageStatsWire(stats)
		}
		writeJSON(w, 200, reply)
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
	// Local metering comes from our own DB, so a provider outage does not hide
	// the token counts the card already knows.
	if stats, err := pg.ProfileUsageStats(p.Name); err == nil {
		reply.Usage = usageStatsWire(stats)
	}
	llmQuotaCache.Lock()
	llmQuotaCache.m[id] = llmQuotaCacheEntry{reply: reply, at: time.Now()}
	llmQuotaCache.Unlock()
	writeJSON(w, 200, reply)
}
