package agent

import (
	"encoding/json"
	"strings"
	"testing"
)

// Regression for the customer-portal.example defects (evidence: the operator's engagement notes):
// (2) probe scripts crashing on expected-empty results, (3) findings refuted by
// source filed without verification, (4) limiter conclusions drawn from a
// nonexistent identity instead of a clean bucket. The discipline lives in a
// code-owned prompt tail so a DB-edited prompt body cannot drop it.
func TestMeasurementSpecOnAllPentestRoles(t *testing.T) {
	sys := map[string]string{
		"worker":    workerSystem("", "", "/data", "/data/tasks/1/i1"),
		"planner":   plannerSystem("goal", "/data", "/data/tasks/1"),
		"mainagent": mainAgentSystem("goal", "/data", "/data/tasks/1"),
	}
	for role, s := range sys {
		for _, marker := range []string{
			"Verification and measurement discipline",
			"exit 0",                // expected-empty probe results are normal, not errors
			"record_fact",           // unverified leads go to facts, not findings
			"per-account vs per-ip", // identify the limiter bucket being measured
			"clean bucket",          // measure from a fresh account / untouched ip
			"Never conclude \"no limiter\"",
		} {
			if !strings.Contains(s, marker) {
				t.Errorf("%s system prompt misses measurement marker %q", role, marker)
			}
		}
	}
}

// Defect 3 gate: report_finding must refuse a filing without verification, and the
// finding must not be persisted in that case.
func TestReportFindingRequiresVerification(t *testing.T) {
	d := testDB(t)
	defer d.Close()
	task, err := d.CreateTask("verification gate", "fixture", nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer d.DeleteTask(task.ID)
	ts := NewToolSet(nil, "")
	ts.ts, ts.taskID = d.Exploration(task.ExplorationID), task.ID
	res, err := ts.addFinding().Call(t.Context(), json.RawMessage(`{"vulnclass":"AUTH","severity":"high","summary":"unverified claim","evidence":"some output"}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if flat := res.Flatten(); !strings.Contains(flat, "requires verification") {
		t.Fatalf("missing verification not refused: %s", flat)
	}
	if ts.writes.Findings != 0 {
		t.Fatal("unverified finding was counted")
	}
}
