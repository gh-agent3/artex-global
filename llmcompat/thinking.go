// Package llmcompat holds shims for provider quirks that are not worth pushing
// into the shared SDK: today, backends that reject an explicit disabled-thinking
// flag on helper calls (summaries, judges) instead of ignoring it.
package llmcompat

import (
	"context"
	"strings"

	"github.com/Autumn-27/norma/llm"
)

// thinkingUnsupported reports whether err says the request was refused only
// because thinking was explicitly disabled. Kimi K3.1 Pro answers
// 400 invalid_request_error: "K3.1 Pro does not support disabled thinking mode.
// Please enable thinking and try again, or choose another model."; other
// Anthropic-compatible backends word it the same way.
func thinkingUnsupported(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	if !strings.Contains(s, "thinking") {
		return false
	}
	return strings.Contains(s, "does not support") ||
		strings.Contains(s, "not support") ||
		strings.Contains(s, "unsupported")
}

// WithThinkingFallback re-runs call once with the thinking field omitted when the
// provider refused the request only because thinking was disabled. Helper calls
// (compaction, side-question summaries, the intercept judge) disable thinking to
// save tokens; that saves nothing when the backend rejects the request outright,
// and the retry keeps the feature working on such a model instead of failing.
//
// Any other error, and a request that never disabled thinking, is returned as-is.
func WithThinkingFallback[T any](ctx context.Context, req llm.CompletionRequest, call func(llm.CompletionRequest) (T, error)) (T, error) {
	v, err := call(req)
	if err == nil || req.Thinking != "disabled" || !thinkingUnsupported(err) {
		return v, err
	}
	retry := req
	retry.Thinking = ""
	return call(retry)
}

// Complete runs one non-streaming completion through WithThinkingFallback.
func Complete(ctx context.Context, prov llm.Provider, req llm.CompletionRequest) (llm.Message, string, llm.Usage, error) {
	type result struct {
		msg   llm.Message
		stop  string
		usage llm.Usage
	}
	r, err := WithThinkingFallback(ctx, req, func(r llm.CompletionRequest) (result, error) {
		msg, stop, usage, cerr := prov.Complete(ctx, r)
		return result{msg: msg, stop: stop, usage: usage}, cerr
	})
	return r.msg, r.stop, r.usage, err
}

// StreamText drains a streaming completion and returns the concatenated text,
// retrying once without the thinking field when the backend rejects it.
func StreamText(ctx context.Context, prov llm.Provider, req llm.CompletionRequest) (string, error) {
	return WithThinkingFallback(ctx, req, func(r llm.CompletionRequest) (string, error) {
		var sb strings.Builder
		for ev, err := range prov.Stream(ctx, r) {
			if err != nil {
				return "", err
			}
			if ev.Type == llm.SETextDelta {
				sb.WriteString(ev.Text)
			}
		}
		return sb.String(), nil
	})
}
