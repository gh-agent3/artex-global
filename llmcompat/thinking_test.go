package llmcompat

import (
	"context"
	"errors"
	"iter"
	"testing"

	"github.com/Autumn-27/norma/llm"
)

// kimiDisabledThinking is the body Kimi K3.1 Pro returns (HTTP 400) when a
// request carries thinking.type=disabled; the SDK surfaces it as this text.
var kimiDisabledThinking = errors.New(`anthropic: status 400: {"error":{"type":"invalid_request_error","message":"K3.1 Pro does not support disabled thinking mode. Please enable thinking and try again, or choose another model."},"type":"error"}`)

type scriptedProvider struct {
	thinking []string // Thinking field of every request, in order
	text     string
}

func (p *scriptedProvider) Stream(ctx context.Context, req llm.CompletionRequest) iter.Seq2[llm.StreamEvent, error] {
	return func(yield func(llm.StreamEvent, error) bool) {
		p.thinking = append(p.thinking, req.Thinking)
		if req.Thinking == "disabled" {
			yield(llm.StreamEvent{}, kimiDisabledThinking)
			return
		}
		yield(llm.StreamEvent{Type: llm.SETextDelta, Text: p.text}, nil)
	}
}

func (p *scriptedProvider) Complete(ctx context.Context, req llm.CompletionRequest) (llm.Message, string, llm.Usage, error) {
	p.thinking = append(p.thinking, req.Thinking)
	if req.Thinking == "disabled" {
		return llm.Message{}, "", llm.Usage{}, kimiDisabledThinking
	}
	return llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentBlock{llm.TextBlock(p.text)}}, "end_turn", llm.Usage{InputTokens: 3, OutputTokens: 4}, nil
}

func TestCompleteRetriesWithoutDisabledThinking(t *testing.T) {
	prov := &scriptedProvider{text: "summary"}
	msg, _, usage, err := Complete(context.Background(), prov, llm.CompletionRequest{
		System: []string{"s"}, Messages: []llm.Message{llm.UserText("u")}, Thinking: "disabled",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.Text() != "summary" || usage.OutputTokens != 4 {
		t.Fatalf("unexpected result: %q %+v", msg.Text(), usage)
	}
	if len(prov.thinking) != 2 || prov.thinking[0] != "disabled" || prov.thinking[1] != "" {
		t.Fatalf("want a disabled attempt then a default one, got %#v", prov.thinking)
	}
}

func TestStreamTextRetriesWithoutDisabledThinking(t *testing.T) {
	prov := &scriptedProvider{text: "streamed"}
	got, err := StreamText(context.Background(), prov, llm.CompletionRequest{Thinking: "disabled"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "streamed" {
		t.Fatalf("got %q", got)
	}
}

func TestThinkingFallbackLeavesOrdinaryErrorsAlone(t *testing.T) {
	boom := errors.New("status 429: rate limit")
	calls := 0
	_, err := WithThinkingFallback(context.Background(), llm.CompletionRequest{Thinking: "disabled"},
		func(llm.CompletionRequest) (string, error) {
			calls++
			return "", boom
		})
	if !errors.Is(err, boom) || calls != 1 {
		t.Fatalf("want the original error after one attempt, got %v after %d", err, calls)
	}
}

func TestThinkingFallbackIgnoresRequestsThatKeepThinking(t *testing.T) {
	calls := 0
	got, err := WithThinkingFallback(context.Background(), llm.CompletionRequest{Thinking: "enabled"},
		func(llm.CompletionRequest) (string, error) {
			calls++
			return "", kimiDisabledThinking
		})
	if err == nil || got != "" || calls != 1 {
		t.Fatalf("a thinking-enabled request must not be retried: got %q %v after %d", got, err, calls)
	}
}

func TestThinkingUnsupportedMatchers(t *testing.T) {
	positive := []string{
		kimiDisabledThinking.Error(),
		`{"error":{"message":"model does not support disabled thinking"}}`,
		`400: thinking disabled is unsupported by this model`,
	}
	for _, s := range positive {
		if !thinkingUnsupported(errors.New(s)) {
			t.Errorf("expected a disabled-thinking rejection: %q", s)
		}
	}
	negative := []string{
		`status 429: rate limit`,
		`status 400: invalid thinking budget`,
		`context length exceeded`,
	}
	for _, s := range negative {
		if thinkingUnsupported(errors.New(s)) {
			t.Errorf("unexpected classification: %q", s)
		}
	}
}
