package server

import (
	"fmt"

	"github.com/gh-agent3/artex-global/config"
)

// face picks the operator-facing string for the active output language.
// English and Russian are explicit. Every other language, including the unwired
// test default, keeps the existing Korean text so the localization golden tests
// stay on their baseline.
func face(en, ru, ko string) string {
	switch config.ActiveLanguage() {
	case "en":
		return en
	case "ru":
		return ru
	default:
		return ko
	}
}

func goalBreakdownRound0Text() string {
	return face("Goal breakdown, round 0", "Разбор целей, раунд 0", goalBreakdownRound0Summary)
}

func plannerRoundSummary(n int) string {
	switch config.ActiveLanguage() {
	case "en":
		return fmt.Sprintf("Planning round %d", n)
	case "ru":
		return fmt.Sprintf("Раунд планирования %d", n)
	default:
		return fmt.Sprintf(plannerRoundSummaryFmt, n)
	}
}

func sourceNameGlobalConfigText() string {
	return face("Global settings", "Глобальные настройки", sourceNameGlobalConfig)
}
