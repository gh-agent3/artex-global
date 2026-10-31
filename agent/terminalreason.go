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
	harness.ReasonCompleted:         "the model ended the turn normally but left no text summary; the facts and assets are taken from the tool calls of this round",
	harness.ReasonMaxTurns:          "hit the step cap (MaxTurns): the SDK settled and wrote back facts and assets, and the intent is marked exhausted so the planner can continue in another direction instead of treating it as a failure",
	harness.ReasonTimeout:           "hit the wall-clock budget of a single run (MaxDuration): the running tool is interrupted at the deadline and the run settles in place, writing back the facts and assets identified so far; the intent is marked exhausted",
	harness.ReasonModelError:        "the model or API call failed (network, auth, rate limit, provider 5xx and so on); once the retries ran out the intent is marked blocked - a transport fault means this intent barely got tested, so read its trace (get_worker_trace) before redispatching or changing approach",
	harness.ReasonBlockingLimit:     "the context reached its hard length limit and the request was blocked before it was sent; narrow the intent granularity or compact the tool output",
	harness.ReasonPromptTooLong:     "the prompt was too long and the context-compaction retries are exhausted, so the run cannot continue",
	harness.ReasonImageError:        "the current model does not accept multimodal content in this round; switch to a vision-capable model or cap the images in tool results",
	harness.ReasonStopHookPrevented: "a Stop hook prevented this turn from ending and the run could not continue; check whether the task Guard rules are too strict",
	harness.ReasonHookStopped:       "a tool or hook stopped further execution on purpose, e.g. an out-of-scope target or a forbidden command; check the block note in the last tool_result",
	harness.ReasonAbortedStreaming:  "the run was cancelled while the model was streaming its output",
	harness.ReasonAbortedTools:      "the run was cancelled during tool execution",
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
		return "the context of this run was cancelled but no Terminal event was produced underneath"
	}
	return "unknown terminal state; the harness may have added a TerminalReason - extend reasonHint"
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
