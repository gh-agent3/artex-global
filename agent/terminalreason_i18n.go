package agent

import "github.com/Autumn-27/norma/harness"

// User-facing terminal notices were written in Chinese and ignored the output
// language, so a pause always rendered the Chinese block. zh keeps those
// strings. en and ru are the localized forms. Other languages fall back to en
// until they have their own copy, instead of leaking Chinese.

type termPhrase struct {
	noCauseShort   string
	stageDuring    string
	stageStreaming string
	stageTools     string
	sumAbort       string
	sumBudget      string
	sumOther       string
	progressPrefix string
	turns          string
	labelState     string
	labelCause     string
	noCauseDetail  string
	labelErr       string
	partialOut     string
	executed       string
	elapsed        string
	tokens         string
	noTools        string
	hangingTool    string
	lastTool       string
	emptyHint      string
	unknownHint    string
}

func terminalDisplayLang() string {
	if OutputLanguage == nil {
		return "zh"
	}
	switch resolveOutputLang() {
	case "zh", "ru":
		return resolveOutputLang()
	default:
		return "en"
	}
}

func termPhraseOf(lang string) termPhrase {
	if lang == "ru" {
		return termPhraseRu
	}
	if lang == "en" {
		return termPhraseEn
	}
	return termPhraseZh
}

var termPhraseZh = termPhrase{
	noCauseShort:   "no cancellation cause available",
	stageDuring:    "during execution",
	stageStreaming: "during model output",
	stageTools:     "during tool execution",
	sumAbort:       "(run interrupted: %s; stopped %s%s, not finished)",
	sumBudget:      "(hit the run budget cap (%s); settled and wrote back facts%s; no text summary this time)",
	sumOther:       "(no text summary, terminal state %s: %s)",
	progressPrefix: "; ran for ",
	turns:          "%d turns",
	labelState:     "terminal state",
	labelCause:     "cancellation cause",
	noCauseDetail:  "unavailable; the canceller may not have attached a named cause through context.WithCancelCause",
	labelErr:       "underlying error",
	partialOut:     "partial output produced before the cancellation",
	executed:       "- **Turns executed**: %d model turns\n",
	elapsed:        "- **Run duration**: %s\n",
	tokens:         "- **Total tokens**: input %d / output %d / cache read %d / cache write %d\n",
	noTools:        "- **Tool calls**: the run ended before making any tool call\n",
	hangingTool:    "- **Tool running when interrupted**: `%s` (running for %s, **no result returned**)\n\n  ```json\n  %s\n  ```\n",
	lastTool:       "- **Last tool before the interruption**: `%s` (returned normally)\n",
	emptyHint:      "the context of this run was cancelled but no Terminal event was produced",
	unknownHint:    "unknown terminal state; the harness may have added a TerminalReason - extend reasonHint",
}

var termPhraseEn = termPhrase{
	noCauseShort:   "cancellation cause was not recorded",
	stageDuring:    "during execution",
	stageStreaming: "while the model was writing",
	stageTools:     "while a tool was running",
	sumAbort:       "(run interrupted: %s; stopped %s%s, not finished)",
	sumBudget:      "(hit the run budget (%s), wrote back the facts found so far%s; no text summary)",
	sumOther:       "(no text summary, terminal state %s: %s)",
	progressPrefix: ", ran ",
	turns:          "%d turns",
	labelState:     "Terminal state",
	labelCause:     "Interrupt cause",
	noCauseDetail:  "unavailable; the canceller did not attach a named cause via context.WithCancelCause",
	labelErr:       "Underlying error",
	partialOut:     "Partial output written before cancellation",
	executed:       "- **Executed**: %d model turns\n",
	elapsed:        "- **Run time**: %s\n",
	tokens:         "- **Tokens**: input %d / output %d / cache read %d / cache write %d\n",
	noTools:        "- **Tool calls**: the run ended before any tool call\n",
	hangingTool:    "- **Tool in flight at interrupt**: `%s` (ran %s, **no result returned**)\n\n  ```json\n  %s\n  ```\n",
	lastTool:       "- **Last tool before interrupt**: `%s` (returned normally)\n",
	emptyHint:      "the run context was cancelled, but the harness emitted no Terminal event",
	unknownHint:    "unknown terminal state; the harness may have added a TerminalReason that is not described yet",
}

var termPhraseRu = termPhrase{
	noCauseShort:   "причина отмены не записана",
	stageDuring:    "во время выполнения",
	stageStreaming: "на этапе ответа модели",
	stageTools:     "на этапе вызова инструмента",
	sumAbort:       "(запуск прерван: %s; остановка %s%s, не завершён)",
	sumBudget:      "(достигнут предел запуска (%s), уже найденные факты записаны%s; текстового итога нет)",
	sumOther:       "(нет текстового итога, конечное состояние %s: %s)",
	progressPrefix: ", уже шло ",
	turns:          "%d ход.",
	labelState:     "Конечное состояние",
	labelCause:     "Причина прерывания",
	noCauseDetail:  "не удалось получить; отменяющая сторона не передала именованную причину через context.WithCancelCause",
	labelErr:       "Нижележащая ошибка",
	partialOut:     "Частичный вывод до отмены",
	executed:       "- **Выполнено**: %d ход. модели\n",
	elapsed:        "- **Длительность этого запуска**: %s\n",
	tokens:         "- **Токены**: вход %d / выход %d / чтение кэша %d / запись кэша %d\n",
	noTools:        "- **Вызовы инструментов**: запуск закончился до первого вызова\n",
	hangingTool:    "- **Инструмент ещё выполнялся**: `%s` (шёл %s, **результат не вернулся**)\n\n  ```json\n  %s\n  ```\n",
	lastTool:       "- **Последний инструмент до прерывания**: `%s` (завершился нормально)\n",
	emptyHint:      "context запуска отменён, но harness не прислал событие Terminal",
	unknownHint:    "неизвестное конечное состояние; в harness мог появиться TerminalReason без описания",
}

func localizeAbort(lang, code, short, text string) (string, string) {
	if lang == "zh" {
		return short, text
	}
	if row, ok := abortL10n[lang][code]; ok {
		return row[0], row[1]
	}
	if row, ok := abortL10n["en"][code]; ok {
		return row[0], row[1]
	}
	return short, text
}

func localizeHint(lang string, reason harness.TerminalReason) string {
	if lang == "zh" {
		return terminalReasonHint(reason)
	}
	if hint := hintL10n[lang][reason]; hint != "" {
		return hint
	}
	if hint := hintL10n["en"][reason]; hint != "" {
		return hint
	}
	p := termPhraseOf(lang)
	if reason == "" {
		return p.emptyHint
	}
	return p.unknownHint
}
