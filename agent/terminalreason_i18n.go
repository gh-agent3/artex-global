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
	noCauseShort:   "未能取得取消原因",
	stageDuring:    "执行过程中",
	stageStreaming: "模型输出阶段",
	stageTools:     "工具执行阶段",
	sumAbort:       "（运行被中断：%s；停在%s%s，未完成）",
	sumBudget:      "（达到运行预算上限(%s)，已收尾写回事实%s；本次无文字总结）",
	sumOther:       "（无文字总结，终态 %s：%s）",
	progressPrefix: "，已运行 ",
	turns:          "%d 轮",
	labelState:     "终态",
	labelCause:     "中断原因",
	noCauseDetail:  "无法取得；取消方可能没有通过 context.WithCancelCause 附加具名原因",
	labelErr:       "底层错误",
	partialOut:     "取消前已生成的部分输出",
	executed:       "- **已执行**: %d 轮模型回合\n",
	elapsed:        "- **本次运行耗时**: %s\n",
	tokens:         "- **累计 token**: 输入 %d / 输出 %d / 缓存读 %d / 缓存写 %d\n",
	noTools:        "- **工具调用**: 本次运行还没有发出工具调用就结束了\n",
	hangingTool:    "- **中断时正在执行的工具**: `%s`（已运行 %s，**未返回结果**）\n\n  ```json\n  %s\n  ```\n",
	lastTool:       "- **中断前最后一个工具**: `%s`（已正常返回）\n",
	emptyHint:      "运行的 context 已取消，但底层没有产生 Terminal 事件",
	unknownHint:    "未知终态；harness 可能新增了 TerminalReason，请补充 reasonHint",
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
