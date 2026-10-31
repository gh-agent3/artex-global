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
