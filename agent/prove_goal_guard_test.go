package agent

import (
	"strings"
	"testing"
)

// TestProveGoalNegationMatcher pins the matcher used to refuse a prove_goal call
// whose own reason disowns the proof. The planner used prove_goal as a note-taking
// tool on a real task and wrote "NOT MET - do not treat as proof ... goal state
// should remain open"; the tool marked the goal met and closed the task anyway.
func TestProveGoalNegationMatcher(t *testing.T) {
	refuse := []string{
		"NOT MET - do not treat as proof. This evidence only closes the signed-upload chapter; no command execution has been demonstrated against the target. (Recorded for traceability of the furthest progress only; goal state should remain open.)",
		"not met yet",
		"the goal is still unmet",
		"no evidence of execution",
		"this does not prove the goal",
		"no command execution was observed",
		"goal state should remain open",
	}
	for _, r := range refuse {
		if !proveGoalNegation.MatchString(r) {
			t.Errorf("a disowned proof must be refused: %q", r)
		}
	}
	accept := []string{
		"finding 42 shows the unauthenticated RCE and the id output",
		"the worker got uid=0 on the target via the signed upload",
		"coverage reached 100% (measured 100.0) with 12 assets tested",
		"",
	}
	for _, r := range accept {
		if proveGoalNegation.MatchString(r) {
			t.Errorf("an affirmative reason must not be refused: %q", r)
		}
	}
}

// TestProveGoalRefusesDisownedProof checks the guard returns the record_fact hint
// for a disowned proof and nil for an affirmative one.
func TestProveGoalRefusesDisownedProof(t *testing.T) {
	err := disownedProofError("NOT MET - do not treat as proof; no command execution has been demonstrated against the target")
	if err == nil || !strings.Contains(err.Error(), "record_fact") {
		t.Fatalf("a disowned proof must be refused and point at record_fact, got %v", err)
	}
	if err := disownedProofError("finding 42 shows the unauthenticated RCE with the id output"); err != nil {
		t.Fatalf("an affirmative reason must pass, got %v", err)
	}
}
