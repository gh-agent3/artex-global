package server

import (
	"strings"
	"testing"

	"artex-global/config"
)

func TestOperatorChromeFollowsEnglishAndRussian(t *testing.T) {
	prev := config.CurrentLanguage
	t.Cleanup(func() { config.CurrentLanguage = prev })

	cases := []struct {
		lang string
		want []string
		ban  []string
	}{
		{"en", []string{"Goal breakdown, round 0", "Planning round 1", "Global settings"}, []string{"전역 설정", "0차", "1차", "运行被中断"}},
		{"ru", []string{"Разбор целей, раунд 0", "Раунд планирования 1", "Глобальные настройки"}, []string{"전역 설정", "0차", "运行被中断", "Global settings"}},
	}
	for _, tc := range cases {
		config.CurrentLanguage = func() string { return tc.lang }
		got := []string{
			goalBreakdownRound0Text(),
			plannerRoundSummary(1),
			sourceNameGlobalConfigText(),
		}
		blob := strings.Join(got, "\n")
		for _, want := range tc.want {
			if !strings.Contains(blob, want) {
				t.Errorf("%s missing %q in %q", tc.lang, want, blob)
			}
		}
		for _, ban := range tc.ban {
			if strings.Contains(blob, ban) {
				t.Errorf("%s leaked %q in %q", tc.lang, ban, blob)
			}
		}
	}

	config.CurrentLanguage = nil
	if goalBreakdownRound0Text() != goalBreakdownRound0Summary {
		t.Fatalf("unwired language must keep the Korean baseline, got %q", goalBreakdownRound0Text())
	}
}
