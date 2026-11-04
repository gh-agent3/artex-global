// Package notify implements the IM / email delivery adapters for findings.
//
// Layering: this is a **leaf package** that depends only on the standard library.
// It knows nothing about the database or the server. Channel config arrives as
// map[string]any (the notification_channels.config JSONB column) and the content to
// push arrives as Message. The split keeps the genuinely error-prone parts --
// signature computation, UTF-8 truncation, filter matching -- unit-testable without
// PostgreSQL, while the host only orchestrates on the server side.
//
// Concurrency: Channel implementations must be **stateless**. A single Channel
// instance is reused concurrently by several channel configs (even several bots of
// the same kind), so all credentials come from the cfg argument; never cache things
// like the webhook URL in the implementation's own fields.
package notify

// Channel kind identifiers. The values are also the valid set for
// notification_channels.kind, checked against a server-side whitelist (same as
// findings.status: no DB CHECK, so adding a channel later stays easy).
const (
	KindDingTalk = "dingtalk" // DingTalk custom robot
	KindFeishu   = "feishu"   // Feishu (incl. Lark) custom robot
	KindWeCom    = "wecom"    // WeCom group robot
	KindWebhook  = "webhook"  // Generic webhook: custom method/headers/JSON template
	KindTelegram = "telegram" // Telegram Bot API
	KindEmail    = "email"    // SMTP email
)

// Event kinds, matching notification_events.kind.
const (
	EventFindingCreated       = "finding_created"
	EventFindingStatusChanged = "finding_status_changed"
)

// InitKind is the fallback for an empty kind in config.
const InitKind = KindDingTalk

// severityRank maps a severity to a comparable ordinal. Unknown severities return 0,
// so any min_severity setting filters them out -- when in doubt, do not push, which
// keeps false positives from flooding the chat.
var severityRank = map[string]int{
	"low":      1,
	"medium":   2,
	"high":     3,
	"critical": 4,
}

// SeverityRank returns the severity ordinal; unknown severities return 0.
func SeverityRank(severity string) int { return severityRank[severity] }

// Lang, when wired by the server, returns the active user-facing output language
// ("en"/"ko"/"zh"/"es"/"ja"/"ru") for the strings this leaf package renders into push /
// email messages (severity and status labels). Kept as an injected func so the
// package stays standard-library-only: it does not import config/db/server. nil
// (unit tests / standalone) → labelLang falls back to "ko", preserving the Korean
// localization golden tests.
var Lang func() string

func labelLang() string {
	if Lang != nil {
		switch v := Lang(); v {
		case "en", "ko", "zh", "es", "ja", "ru":
			return v
		}
	}
	return "ko"
}

// severityLabels / statusLabels hold the per-language display strings. "ko" keeps
// the exact Korean values the localization golden tests pin.
var severityLabels = map[string]map[string]string{
	"ko": {"critical": "🔴 심각", "high": "🟠 높음", "medium": "🟡 중간", "low": "🔵 낮음"},
	"en": {"critical": "🔴 Critical", "high": "🟠 High", "medium": "🟡 Medium", "low": "🔵 Low"},
	"zh": {"critical": "🔴 严重", "high": "🟠 高危", "medium": "🟡 中危", "low": "🔵 低危"},
	"es": {"critical": "🔴 Crítico", "high": "🟠 Alto", "medium": "🟡 Medio", "low": "🔵 Bajo"},
	"ja": {"critical": "🔴 重大", "high": "🟠 高", "medium": "🟡 中", "low": "🔵 低"},
	"ru": {"critical": "🔴 Критический", "high": "🟠 Высокий", "medium": "🟡 Средний", "low": "🔵 Низкий"},
}

var statusLabels = map[string]map[string]string{
	"ko": {"pending": "처리 대기", "in_progress": "처리 중", "confirmed": "확인됨", "resolved": "처리됨", "fixed": "수정됨", "false_positive": "오탐", "ignored": "무시", "duplicate": "중복", "risk_accepted": "위험 수용"},
	"en": {"pending": "Pending", "in_progress": "In progress", "confirmed": "Confirmed", "resolved": "Resolved", "fixed": "Fixed", "false_positive": "False positive", "ignored": "Ignored", "duplicate": "Duplicate", "risk_accepted": "Risk accepted"},
	"zh": {"pending": "待处理", "in_progress": "处理中", "confirmed": "已确认", "resolved": "已处理", "fixed": "已修复", "false_positive": "误报", "ignored": "忽略", "duplicate": "重复", "risk_accepted": "接受风险"},
	"es": {"pending": "Pendiente", "in_progress": "En curso", "confirmed": "Confirmado", "resolved": "Resuelto", "fixed": "Corregido", "false_positive": "Falso positivo", "ignored": "Ignorado", "duplicate": "Duplicado", "risk_accepted": "Riesgo aceptado"},
	"ja": {"pending": "対応待ち", "in_progress": "対応中", "confirmed": "確認済み", "resolved": "対応済み", "fixed": "修正済み", "false_positive": "誤検知", "ignored": "無視", "duplicate": "重複", "risk_accepted": "リスク受容"},
	"ru": {"pending": "Ожидает", "in_progress": "В работе", "confirmed": "Подтверждено", "resolved": "Закрыто", "fixed": "Исправлено", "false_positive": "Ложное срабатывание", "ignored": "Игнорируется", "duplicate": "Дубликат", "risk_accepted": "Риск принят"},
}

// SeverityLabel returns the emoji-prefixed severity name in the current output
// language, used in message titles and card colors. Unknown severities are echoed
// unchanged, never invented.
func SeverityLabel(severity string) string {
	if v, ok := severityLabels[labelLang()][severity]; ok {
		return v
	}
	return severity
}

// StatusLabel translates a triage status into the current output language, used in
// status-change messages. Unknown statuses are echoed unchanged.
func StatusLabel(status string) string {
	if v, ok := statusLabels[labelLang()][status]; ok {
		return v
	}
	return status
}

// AtLeast reports whether severity meets the min threshold. An empty min means no
// threshold: everything passes. Note that an unknown severity has ordinal 0 and is
// rejected by any non-empty min (see the severityRank comment).
func AtLeast(severity, min string) bool {
	if min == "" {
		return true
	}
	return SeverityRank(severity) >= SeverityRank(min)
}
