package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Autumn-27/norma/harness"
)

// runTrace retains the latest tool call so an interrupted run can identify the
// operation that was still in flight.
type runTrace struct {
	startedAt time.Time
	id        string
	name      string
	input     string
	at        time.Time
	pending   bool
}

func (t *runTrace) start(id, name, input string) {
	t.id, t.name, t.input, t.at, t.pending = id, name, input, time.Now(), true
}

func (t *runTrace) done(id string) {
	if id == t.id {
		t.pending = false
	}
}

var reasonHint = map[harness.TerminalReason]string{
	harness.ReasonCompleted:         "模型正常结束了本轮，但没有留下文字总结；事实和资产以本轮工具调用记录为准",
	harness.ReasonMaxTurns:          "达到步数上限(MaxTurns)：SDK 已执行收尾并写回事实和资产，意图会标记为 exhausted，供规划者换方向继续，而不是作为失败处理",
	harness.ReasonTimeout:           "达到单次运行的墙钟预算(MaxDuration)：到点会打断在跑的工具并就地进收尾，把已识别的事实和资产写回，意图会标记为 exhausted",
	harness.ReasonModelError:        "模型或 API 调用失败（网络、鉴权、限流、供应商 5xx 等），重试用尽后意图标记为 blocked——传输层故障导致这条意图基本没真正探成；查其执行过程（get_worker_trace）后再决定重派或换法",
	harness.ReasonBlockingLimit:     "上下文长度达到硬上限，请求在发出前被拦截；应收窄意图粒度或压缩工具返回",
	harness.ReasonPromptTooLong:     "提示词过长且上下文压缩重试已经用尽，无法继续执行",
	harness.ReasonImageError:        "当前模型不支持本轮多模态内容；请切换支持视觉的模型或避免工具返回图片",
	harness.ReasonStopHookPrevented: "Stop 钩子阻止本轮结束，随后未能继续；请检查任务 Guard 规则是否过严",
	harness.ReasonHookStopped:       "工具或钩子主动停止继续执行，例如越界目标或禁用命令；请检查最后一条 tool_result 的拦截说明",
	harness.ReasonAbortedStreaming:  "运行在模型输出流式生成阶段被取消",
	harness.ReasonAbortedTools:      "运行在工具执行阶段被取消",
}

// terminalText renders a terminal event with no final text into a compact summary
// and a Markdown detail block.
func terminalText(ctx context.Context, term *harness.Terminal, tr *runTrace) (string, string) {
	reason := term.Reason
	aborted := reason == harness.ReasonAbortedStreaming || reason == harness.ReasonAbortedTools
	// Prompt may return ctx.Err directly without a terminal event. Preserve the
	// cancellation cause instead of falling back to an empty/unknown terminal reason.
	if reason == "" && ctx.Err() != nil {
		aborted = true
	}

	lang := terminalDisplayLang()
	p := termPhraseOf(lang)
	code, short, why, hasCause := AbortReason(ctx)
	if hasCause {
		short, why = localizeAbort(lang, code, short, why)
	}

	var sum string
	if aborted {
		if !hasCause {
			short = p.noCauseShort
		}
		stage := p.stageDuring
		switch reason {
		case harness.ReasonAbortedStreaming:
			stage = p.stageStreaming
		case harness.ReasonAbortedTools:
			stage = p.stageTools
		}
		sum = fmt.Sprintf(p.sumAbort, short, stage, progressSuffix(lang, term, tr))
	} else if reason == harness.ReasonMaxTurns || reason == harness.ReasonTimeout {
		sum = fmt.Sprintf(p.sumBudget, string(reason), progressSuffix(lang, term, tr))
	} else {
		hint := localizeHint(lang, reason)
		sum = fmt.Sprintf(p.sumOther, terminalReasonLabel(reason), firstLine(hint, 80))
	}

	var b strings.Builder
	b.WriteString(sum)
	b.WriteString("\n\n")
	displayReason := terminalReasonLabel(reason)
	fmt.Fprintf(&b, "- **%s**: `%s` - %s\n", p.labelState, displayReason, localizeHint(lang, reason))
	if aborted {
		if hasCause {
			fmt.Fprintf(&b, "- **%s** (`%s`): %s\n", p.labelCause, code, why)
		} else {
			fmt.Fprintf(&b, "- **%s**: %s\n", p.labelCause, p.noCauseDetail)
		}
	}
	if term.Err != nil {
		fmt.Fprintf(&b, "- **%s**: `%v`\n", p.labelErr, term.Err)
	}
	if aborted && strings.TrimSpace(term.Text) != "" {
		fmt.Fprintf(&b, "- **%s**:\n\n", p.partialOut)
		b.WriteString(term.Text)
		b.WriteString("\n\n")
	}
	if term.Turns > 0 {
		fmt.Fprintf(&b, p.executed, term.Turns)
	}
	if !tr.startedAt.IsZero() {
		fmt.Fprintf(&b, p.elapsed, roundDur(time.Since(tr.startedAt)))
	}
	if u := term.Usage; u.InputTokens+u.OutputTokens+u.CacheReadTokens+u.CacheWriteTokens > 0 {
		fmt.Fprintf(&b, p.tokens, u.InputTokens, u.OutputTokens, u.CacheReadTokens, u.CacheWriteTokens)
	}
	if tr.name == "" {
		b.WriteString(p.noTools)
	} else if tr.pending {
		fmt.Fprintf(&b, p.hangingTool, tr.name, roundDur(time.Since(tr.at)), firstLine(tr.input, 300))
	} else {
		fmt.Fprintf(&b, p.lastTool, tr.name)
	}
	return sum, b.String()
}

func terminalReasonLabel(reason harness.TerminalReason) string {
	if reason == "" {
		return "context_canceled"
	}
	return string(reason)
}

func terminalReasonHint(reason harness.TerminalReason) string {
	if hint := reasonHint[reason]; hint != "" {
		return hint
	}
	if reason == "" {
		return "运行的 context 已取消，但底层没有产生 Terminal 事件"
	}
	return "未知终态；harness 可能新增了 TerminalReason，请补充 reasonHint"
}

func progressSuffix(lang string, term *harness.Terminal, tr *runTrace) string {
	p := termPhraseOf(lang)
	var parts []string
	if term.Turns > 0 {
		parts = append(parts, fmt.Sprintf(p.turns, term.Turns))
	}
	if !tr.startedAt.IsZero() {
		parts = append(parts, roundDur(time.Since(tr.startedAt)))
	}
	if len(parts) == 0 {
		return ""
	}
	return p.progressPrefix + strings.Join(parts, " / ")
}

func roundDur(d time.Duration) string {
	switch {
	case d < time.Minute:
		return d.Round(100 * time.Millisecond).String()
	case d < time.Hour:
		return d.Round(time.Second).String()
	default:
		return d.Round(time.Minute).String()
	}
}
