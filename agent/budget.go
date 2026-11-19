package agent

// Hierarchical run budgets (off by default) — trimmed port of BaaRai/ARTEX
// agent/budget.go. A task carries an optional cap on metered LLM tokens and
// turns; when a capped dimension drops to the wrap-up reserve, the engine
// stops dispatching new work and settles the task through one final planner
// round (the same endgame as the wall-clock timeout). Counting matches the
// llm_usage ledger: one turn per metered call, tokens = input + output.

import "math"

// Unlimited is the cap of a dimension with no budget (0 or unset).
const Unlimited int64 = math.MaxInt64

const (
	BudgetMarginPercent         = 5      // wrap-up reserve = max(5% of the task budget, floor)
	BudgetMarginMinTokens       = 20_000 // token floor of the reserve
	BudgetMarginMinTurns        = 2      // turn floor of the reserve
	DefaultWorkerEstimateTokens = 50_000 // one worker run, until a worker of this task finished
	DefaultWorkerEstimateTurns  = 10
)

// Budget caps tokens and turns; a dimension equal to Unlimited has no cap.
type Budget struct{ Tokens, Turns int64 }

// Spend is what a level has consumed.
type Spend struct{ Tokens, Turns int64 }

// NewBudget builds a budget from configured values: <=0 means Unlimited.
func NewBudget(tokens, turns int64) Budget { return Budget{budgetCap(tokens), budgetCap(turns)} }

func budgetCap(n int64) int64 {
	if n <= 0 {
		return Unlimited
	}
	return n
}

// Remaining is the budget left after s (never negative; Unlimited stays Unlimited).
func (b Budget) Remaining(s Spend) Budget {
	return Budget{left(b.Tokens, s.Tokens), left(b.Turns, s.Turns)}
}

func left(capacity, spent int64) int64 {
	if capacity == Unlimited {
		return Unlimited
	}
	return max(capacity-spent, 0)
}

// WrapupMargin is the reserve kept for the forced final round (0 when unlimited).
func WrapupMargin(task Budget) Budget {
	return Budget{
		Tokens: margin(task.Tokens, BudgetMarginPercent, BudgetMarginMinTokens),
		Turns:  margin(task.Turns, BudgetMarginPercent, BudgetMarginMinTurns),
	}
}

func margin(capacity, percent, floor int64) int64 {
	if capacity == Unlimited {
		return 0
	}
	m := capacity * percent / 100
	if m < floor {
		m = floor
	}
	if m > capacity {
		m = capacity
	}
	return m
}

// Exhausted reports whether a capped dimension has burned into the wrap-up
// reserve of THIS (task-level) budget after s was spent — i.e. the task can
// no longer afford a fresh worker run and the engine should stop dispatching
// and settle through the final round.
func (task Budget) Exhausted(spend Spend) bool {
	remaining := task.Remaining(spend)
	m := WrapupMargin(task)
	tokensOut := task.Tokens != Unlimited && remaining.Tokens <= m.Tokens
	turnsOut := task.Turns != Unlimited && remaining.Turns <= m.Turns
	return tokensOut || turnsOut
}

// Unlimited reports whether neither dimension is capped.
func (b Budget) Unlimited() bool { return b.Tokens == Unlimited && b.Turns == Unlimited }
