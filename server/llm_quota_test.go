package server

import "testing"

func TestKimiUsagesURL(t *testing.T) {
	cases := []struct {
		base string
		want string
	}{
		{"https://api.kimi.com/coding", "https://api.kimi.com/coding/v1/usages"},
		{"https://api.kimi.com/coding/", "https://api.kimi.com/coding/v1/usages"},
		{"https://api.kimi.ai/coding", "https://api.kimi.ai/coding/v1/usages"},
		{"https://api.kimi.com/coding/v1", ""}, // a /v1 suffix would double the segment
		{"https://api.experientiallabs.ai/v1", ""},
		{"http://172.17.0.1:4012/v1", ""},
		{"", ""},
		{"://bad", ""},
	}
	for _, c := range cases {
		if got := kimiUsagesURL(c.base); got != c.want {
			t.Errorf("kimiUsagesURL(%q) = %q, want %q", c.base, got, c.want)
		}
	}
}

func TestGrokQuotaURL(t *testing.T) {
	ep, direct := grokQuotaURL("http://172.17.0.1:4012/v1", "grok-4.7")
	if ep != "http://172.17.0.1:4012/v1/grok/quota" || direct {
		t.Fatalf("got (%q, %v), want router quota URL", ep, direct)
	}
	ep, direct = grokQuotaURL("https://api.x.ai/v1", "grok-4.7")
	if ep != "https://cli-chat-proxy.grok.com/v1/billing?format=credits" || !direct {
		t.Fatalf("got (%q, %v), want direct xAI billing URL", ep, direct)
	}
	if ep, _ := grokQuotaURL("http://172.17.0.1:4012/v1", "think"); ep != "" {
		t.Fatalf("non-grok model resolved %q", ep)
	}
}

func TestParseGrokQuotaBody(t *testing.T) {
	raw := []byte(`{"config":{"currentPeriod":{"type":"USAGE_PERIOD_TYPE_WEEKLY","start":"2026-10-31T18:57:25.726+00:00","end":"2026-11-07T18:57:25.726+00:00"},"creditUsagePercent":9.0,"billingPeriodEnd":"2026-11-07T18:57:25.726+00:00"}}`)
	reply, err := parseGrokQuotaBody(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !reply.Supported || len(reply.Quotas) != 1 {
		t.Fatalf("unexpected reply: %+v", reply)
	}
	q := reply.Quotas[0]
	if q.Key != "week" || q.UsedRatio != 0.09 || q.RemainingRatio != 0.91 || q.ResetTime != "2026-11-07T18:57:25.726+00:00" {
		t.Fatalf("unexpected window: %+v", q)
	}
}

func TestChatGPTQuotaURL(t *testing.T) {
	cases := []struct {
		base, model, want string
	}{
		{"http://172.17.0.1:4016/v1", "gpt-6.1-sol", "http://172.17.0.1:4016/v1/quota"},
		{"http://172.17.0.1:4016/v1/responses", "gpt-6.1-sol", "http://172.17.0.1:4016/v1/quota"},
		{"https://chatgpt.com/backend-api/codex", "gpt-6.1-sol", "https://chatgpt.com/backend-api/wham/usage"},
		{"http://172.17.0.1:4016", "gpt-6.1-sol", "http://172.17.0.1:4016/v1/quota"},
		{"http://172.17.0.1:4012/v1", "think", ""},
	}
	for _, c := range cases {
		if got := chatgptQuotaURL(c.base, c.model); got != c.want {
			t.Errorf("chatgptQuotaURL(%q, %q) = %q, want %q", c.base, c.model, got, c.want)
		}
	}
}

func TestParseChatGPTQuotaBody(t *testing.T) {
	raw := []byte(`{"email":"ops@example.com","plan_type":"prolite",
		"rate_limit":{"primary_window":{"used_percent":94,"limit_window_seconds":604800,"reset_at":1793798547},"secondary_window":null},
		"code_review_rate_limit":{"primary_window":{"used_percent":10,"limit_window_seconds":18000,"reset_at":1793790000}},
		"credits":{"has_credits":true,"balance":"12.50"}}`)
	reply, err := parseChatGPTQuotaBody(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !reply.Supported || reply.Plan != "GPT (prolite)" || reply.Account != "ops@example.com" {
		t.Fatalf("unexpected header: %+v", reply)
	}
	if len(reply.Quotas) != 2 {
		t.Fatalf("want 2 windows, got %+v", reply.Quotas)
	}
	week := reply.Quotas[0]
	if week.Key != "week" || week.UsedRatio != 0.94 || week.RemainingRatio < 0.0599 || week.ResetTime == "" {
		t.Fatalf("unexpected weekly window: %+v", week)
	}
	if review := reply.Quotas[1]; review.Key != "code_review" || review.UsedRatio != 0.1 {
		t.Fatalf("unexpected code-review window: %+v", review)
	}
	if len(reply.Balances) != 1 || reply.Balances[0].Total != "12.50" || reply.Balances[0].Currency != "USD" {
		t.Fatalf("unexpected balances: %+v", reply.Balances)
	}
}

func TestParseDeepSeekQuotaBody(t *testing.T) {
	raw := []byte(`{"is_available":true,"balance_infos":[{"currency":"USD","total_balance":"31.58","granted_balance":"0.00","topped_up_balance":"31.58"}]}`)
	reply, err := parseDeepSeekQuotaBody(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(reply.Balances) != 1 {
		t.Fatalf("want one balance, got %+v", reply.Balances)
	}
	b := reply.Balances[0]
	if b.Currency != "USD" || b.Total != "31.58" || b.ToppedUp != "31.58" {
		t.Fatalf("unexpected balance: %+v", b)
	}
	if _, err := parseDeepSeekQuotaBody([]byte(`{"is_available":true,"balance_infos":[]}`)); err == nil {
		t.Fatal("empty balance list must be an error, not a zero quota")
	}
}

func TestDeepSeekQuotaURL(t *testing.T) {
	if got := deepseekQuotaURL("http://172.17.0.1:4012/v1", "think"); got != "http://172.17.0.1:4012/v1/deepseek/quota" {
		t.Fatalf("think profile resolved %q", got)
	}
	if got := deepseekQuotaURL("https://api.deepseek.com/v1", "deepseek-chat"); got != "https://api.deepseek.com/user/balance" {
		t.Fatalf("direct deepseek resolved %q", got)
	}
	if got := deepseekQuotaURL("http://172.17.0.1:4012/v1", "grok-4.7"); got != "" {
		t.Fatalf("grok profile resolved %q", got)
	}
}

func TestQuotaSupportedCoversEveryProvider(t *testing.T) {
	cases := []struct{ base, model string }{
		{"https://api.kimi.com/coding", "k3.1-pro"},
		{"http://172.17.0.1:4012/v1", "grok-4.7"},
		{"http://172.17.0.1:4016/v1", "gpt-6.1-sol"},
		{"http://172.17.0.1:4012/v1", "think"},
	}
	for _, c := range cases {
		if !quotaSupported(c.base, c.model) {
			t.Errorf("quotaSupported(%q, %q) = false", c.base, c.model)
		}
	}
	if quotaSupported("https://api.experientiallabs.ai/v1", "deepseek-v4.1-flash-uncensored") {
		t.Error("a provider with no quota endpoint must report unsupported")
	}
}
