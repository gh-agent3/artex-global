package notify

import (
	"strings"
	"testing"
)

// This file protects the Korean localization of F4 ③ (email body and HTML template) against
// regressions. If the labels in html.go or the validation errors in email.go revert to
// Chinese, this catches it.
// The hasHan / hasHangul / assertKorean helpers live in notify_localized_test.go (same
// package).
//
// All data (title, type, assets, summary) is ASCII. That way the "zero Han in the whole
// output" assertion captures only a regression in the scaffolding labels, not the content.
// Severity and status labels are already Korean since F4 ①, so they appear as Hangul (not
// Han) and pass the assertion.

// TestHTMLItemLabelsLocalized turns on every label branch of a single email body and verifies
// the scaffolding is Korean with no Chinese Han characters.
func TestHTMLItemLabelsLocalized(t *testing.T) {
	m := Message{
		HomeURL: "https://example.com/panel",
		Items: []Item{{
			Name:       "sqli-login", // Title() = Name
			VulnClass:  "injection",  // must differ from Title() so the type line renders
			Severity:   "high",       // SeverityLabel -> 높음 (Hangul)
			Summary:    "login form is injectable",
			Assets:     []string{"host-a.example.com"},
			DetailURL:  "https://example.com/f/1",
			FromStatus: "pending", // IsStatusChange()=true -> status change line
			ToStatus:   "fixed",
		}},
	}
	out := htmlBody(m, 0)
	if hasHan(out) {
		t.Errorf("the email body still contains Chinese Han characters:\n%s", out)
	}
	for _, want := range []string{
		"상태 변경", "유형", "자산", "개요", "상세 보기", "플랫폼에서 전체 보기",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the email body is missing the %q label:\n%s", want, out)
		}
	}
}

// TestHTMLBatchIntroLocalized verifies both digest header branches (with and without a time
// window) render in Korean.
func TestHTMLBatchIntroLocalized(t *testing.T) {
	items := []Item{{Name: "a", Severity: "high"}, {Name: "b", Severity: "low"}}

	withWindow := htmlBatchIntro(Message{Batch: true, WindowMinutes: 30, Items: items})
	assertKorean(t, "htmlBatchIntro(시간창)", withWindow)
	for _, want := range []string{"최근 30분간", "신규 취약점", "2건"} {
		if !strings.Contains(withWindow, want) {
			t.Errorf("the time-window header is missing %q: %q", want, withWindow)
		}
	}

	noWindow := htmlBatchIntro(Message{Batch: true, WindowMinutes: 0, Items: items})
	assertKorean(t, "htmlBatchIntro(시간창 없음)", noWindow)
	if !strings.Contains(noWindow, "신규 취약점 2건") {
		t.Errorf("the header without a time window must contain %q: %q", "신규 취약점 2건", noWindow)
	}
	if strings.Contains(noWindow, "분간") {
		t.Errorf("there is no time window but '분간' appears: %q", noWindow)
	}
}

// TestEmailValidateLocalized verifies the four SMTP config validation errors are Korean and
// each one tells which field is the problem. These messages are shown verbatim to the user
// configuring the notification channel.
func TestEmailValidateLocalized(t *testing.T) {
	cases := []struct {
		name   string
		cfg    map[string]any
		substr string
	}{
		{"서버 주소 누락", map[string]any{"port": float64(25), "from": "a@b.c", "to": []any{"d@e.f"}}, "SMTP"},
		{"포트 범위 벗어남", map[string]any{"host": "h"}, "포트"},
		{"발신자 누락", map[string]any{"host": "h", "port": float64(25), "to": []any{"d@e.f"}}, "발신자"},
		{"수신자 누락", map[string]any{"host": "h", "port": float64(25), "from": "a@b.c"}, "수신자"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := (emailChannel{}).Validate(tc.cfg)
			if err == nil {
				t.Fatalf("검증이 실패해야 합니다: %v", tc.cfg)
			}
			assertKorean(t, "Validate("+tc.name+")", err.Error())
			if !strings.Contains(err.Error(), tc.substr) {
				t.Errorf("오류 메시지에 %q 가 있어야 합니다, 받은 값 %q", tc.substr, err.Error())
			}
		})
	}
}
