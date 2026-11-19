package server

// Task run-budget coordinator (trimmed port of BaaRai/ARTEX agent/budget.go +
// the deadline-coordinator pattern from engine_timeout.go). A task may carry an
// operator-set cap on metered LLM tokens and turns; when a capped dimension
// burns into the wrap-up reserve, the engine stops dispatching new work and
// settles the task through one final planner round — the same endgame as the
// wall-clock timeout, so a budgeted task never just stalls mid-frontier.

import (
	"context"
	"log"
	"strconv"
	"time"

	"artex-global/agent"
)

const budgetPollInterval = 5 * time.Second

// taskBudget returns the task's configured budget (0/0 = unlimited).
func (e *Engine) taskBudget(t *Task) agent.Budget {
	return agent.NewBudget(t.BudgetTokens, t.BudgetTurns)
}

// budgetExhausted reports whether the task's metered spend has burned into the
// wrap-up reserve of a capped dimension. Unlimited budgets never exhaust.
func (e *Engine) budgetExhausted(t *Task) bool {
	b := e.taskBudget(t)
	if b.Unlimited() {
		return false
	}
	id, err := strconv.ParseInt(t.ID, 10, 64)
	if err != nil {
		return false
	}
	tokens, turns, err := e.m.pg.TaskSpend(id)
	if err != nil {
		log.Printf(logA("[budget] task %s spend: %v"), t.ID, err)
		return false
	}
	return b.Exhausted(agent.Spend{Tokens: tokens, Turns: turns})
}

// startBudgetCoordinator launches the per-task budget watcher once (idempotent),
// only for tasks that actually carry a budget. Mirrors startDeadlineCoordinator.
func (e *Engine) startBudgetCoordinator(ctx context.Context, t *Task) {
	if t == nil || e.taskBudget(t).Unlimited() {
		return
	}
	e.deleteMu.RLock()
	if e.IsDeleting(t.ID) {
		e.deleteMu.RUnlock()
		return
	}
	if _, loaded := e.budgetCoordStarted.LoadOrStore(t.ID, true); loaded {
		e.deleteMu.RUnlock()
		return
	}
	rt := e.registerTaskRoutines(ctx, t.ID, 1)
	e.deleteMu.RUnlock()
	runTaskRoutine(rt, func(loopCtx context.Context) { e.budgetCoordinator(loopCtx, t) })
}

func (e *Engine) budgetCoordinator(ctx context.Context, t *Task) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if isTerminalStatus(e.m.TaskStatus(t.ID)) {
			return
		}
		if e.budgetExhausted(t) {
			log.Printf(logA("[budget] task %s run budget reached, starting settle sequence"), t.ID)
			e.settleTaskAs(ctx, t, "failed", "budget")
			return
		}
		if sleepCtx(ctx, budgetPollInterval) {
			return
		}
	}
}
