package db

// Run-budget lifecycle tests: columns, patch, and the llm_usage spend counter
// that the engine's budget coordinator reads.

import (
	"strconv"
	"testing"
)

func TestTaskBudgetLifecycle(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		t.Skipf("database unavailable (%v) — skipping", err)
	}
	defer d.Close()

	task, err := d.CreateTaskWithOptions("budget-fixture", "goal", TaskCreateOptions{
		BudgetTokens: 100_000, BudgetTurns: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer d.DeleteTask(task.ID)
	if task.BudgetTokens != 100_000 || task.BudgetTurns != 10 {
		t.Fatalf("created task lost its budget: %+v", task)
	}
	re, err := d.GetTask(task.ID)
	if err != nil || re == nil || re.BudgetTokens != 100_000 {
		t.Fatalf("read-back budget: %+v err=%v", re, err)
	}

	// patch: set turns, clear tokens (<=0 = unlimited)
	turns := int64(25)
	zero := int64(0)
	patched, err := d.UpdateTask(task.ID, TaskPatch{BudgetTurns: &turns, BudgetTokens: &zero})
	if err != nil || patched == nil {
		t.Fatalf("patch: %+v err=%v", patched, err)
	}
	if patched.BudgetTurns != 25 || patched.BudgetTokens != 0 {
		t.Fatalf("patched budget: %+v", patched)
	}

	if err := d.EnsureLLMUsageTable(); err != nil {
		t.Fatal(err)
	}
	// spend: zero before any metered call (auto-ensures the table on fresh DBs)
	tokens, calls, err := d.TaskSpend(task.ID)
	if err != nil || tokens != 0 || calls != 0 {
		t.Fatalf("empty spend: %d %d err=%v", tokens, calls, err)
	}
	if err := d.InsertLLMUsage(&LLMUsage{TaskID: strconv.FormatInt(task.ID, 10), Model: "m", InputTokens: 700, OutputTokens: 300}); err != nil {
		t.Fatal(err)
	}
	if err := d.InsertLLMUsage(&LLMUsage{TaskID: strconv.FormatInt(task.ID, 10), Model: "m", InputTokens: 100, OutputTokens: 50}); err != nil {
		t.Fatal(err)
	}
	tokens, calls, err = d.TaskSpend(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if tokens != 1150 || calls != 2 {
		t.Fatalf("spend = %d tokens / %d calls, want 1150/2", tokens, calls)
	}
}
