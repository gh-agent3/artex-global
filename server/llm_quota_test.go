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
