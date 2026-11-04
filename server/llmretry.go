package server

import (
	"time"

	"artex-global/agent"
	"artex-global/db"
)

// Server-side resolution of the retry policy, see docs/llm-retry-design.md. Of the five layers:
//   - connect / empty-response / same-provider safe window "follow the endpoint" and each LLM
//     profile may override the global default (a profile field left empty inherits the global,
//     and an unconfigured global uses the built-in default);
//   - circuit breaker / intent rerun are process-level, with exactly one global value.
//
// The global policy reads one settings row from the DB, and every call site is on a low-frequency
// path (building a provider, work settlement, saving a profile), so another cache layer is not
// worth it; the circuit breaker is the exception - it is read on every failure, so applyRetryPolicy
// pushes it into the Registry.

// retryPolicy reads the global policy; a nil DB yields the zero policy (all
// layers on their built-in defaults).
func (s *Server) retryPolicy() db.LLMRetryPolicy {
	if s.m == nil || s.m.pg == nil {
		return db.LLMRetryPolicy{}
	}
	return s.m.pg.LLMRetryPolicy()
}

// resolveRetry layers one profile's override on top of the global policy and
// converts the result into the form agent.Config carries. Rules combine field by
// field, so a profile that only pins an interval still inherits the global count.
func resolveRetry(o db.RetryOverride, pol db.LLMRetryPolicy) agent.RetryConfig {
	connect := o.Connect.Or(pol.Connect)
	empty := o.Empty.Or(pol.Empty)
	stream := o.Stream.Or(pol.Stream)
	return agent.RetryConfig{
		// The count keeps its raw semantics here ("0 = default / negative = off"): the SDK's MaxRetries /
		// EmptyResponseRetries are exactly isomorphic, so it can parse them itself.
		ConnectAttempts: connect.Attempts, ConnectInterval: connect.Interval(),
		EmptyAttempts: empty.Attempts, EmptyInterval: empty.Interval(),
		StreamAttempts: stream.Attempts, StreamInterval: stream.Interval(),
	}
}

// applyProfileRetry fills cfg.Retry for a profile read from the DB.
func (s *Server) applyProfileRetry(cfg *agent.Config, p *db.LLMProfile) {
	if p == nil {
		return
	}
	cfg.Retry = resolveRetry(p.Retry, s.retryPolicy())
}

// The defaults of the circuit breaker (polling cooldown), the same as llmpool's built-in ones - this
// only overrides when the user configured a value. The intent-rerun defaults are in engine.go
// (modelErrorRetries / modelErrorRetryBackoff).

// applyRetryPolicy pushes the process-wide layers of the policy into the objects
// that consume them on a hot path: the circuit-breaker registry. Called at
// startup and whenever the policy is saved.
func (s *Server) applyRetryPolicy() {
	pol := s.retryPolicy()
	if s.llmHealth != nil {
		s.llmHealth.SetPolicy(pol.Breaker.Attempts, pol.Breaker.Interval())
	}
}

// modelErrorRetryPolicy resolves the intent-level replay knobs (layer ⑤): how
// many times a model_error work is re-run and how long to back off between runs.
func (e *Engine) modelErrorRetryPolicy() (retries int, backoff time.Duration) {
	retries, backoff = modelErrorRetries, modelErrorRetryBackoff
	if e == nil || e.m == nil || e.m.pg == nil {
		return retries, backoff
	}
	rule := e.m.pg.LLMRetryPolicy().Intent
	if rule.Attempts != 0 {
		retries = max(rule.Attempts, 0)
	}
	if d := rule.Interval(); d > 0 {
		backoff = d
	}
	return retries, backoff
}

// emptyTurnNudgeLimit resolves how many empty-turn continuations one work may
// inject (see steerHooks.Stop). It deliberately reuses layer 2's knob - the empty-response retry
// count: both are two means of the same thing. The SDK layer governs "not a single content block"
// and resends the same request verbatim; this layer governs "only thinking, no text and no tool" and
// appends one instruction so the model carries on with the reasoning it has (resending is pointless
// for an idle turn caused by the shape of the context). The emptiness definitions differ because the
// SDK goes by "did we yield any event" and a thinking delta is itself an event - but when the user
// configures "retry N times on an empty response" they mean "if the model produced nothing of
// substance, run it again", so the two layers share one count.
//
// It reads the global policy, not a profile override: a run may fail over to another profile mid-way,
// and this is the total cap for the whole intent, which must not change with the endpoint. Same
// semantics as the SDK's emptyRetries(): 0 = default defaultEmptyTurnNudges; -1 (negative) = turn
// off the idle-turn nudge; >0 = use this value.
func (e *Engine) emptyTurnNudgeLimit() int {
	if e == nil || e.m == nil || e.m.pg == nil {
		return defaultEmptyTurnNudges
	}
	switch n := e.m.pg.LLMRetryPolicy().Empty.Attempts; {
	case n == 0:
		return defaultEmptyTurnNudges
	case n < 0:
		return 0
	default:
		return n
	}
}
