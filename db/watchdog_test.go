package db

// Intent watchdog tests: stale detection and the atomic reset guard.

import (
	"testing"
)

func TestStaleRunningIntentReset(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		t.Skipf("database unavailable (%v) — skipping", err)
	}
	defer d.Close()

	task, err := d.CreateTask("watchdog-fixture", "goal", nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer d.DeleteTask(task.ID)

	// created_at is immutable, so insert the intent directly with an old stamp;
	// the "fresh intent" case is covered by the created_at bound below.
	var intent int64
	err = d.QueryRow(`INSERT INTO exploration_nodes(exploration_id, kind, payload, state, origin, created_at)
VALUES ($1,'intent','{"summary":"watchdog fixture"}','running','planner', now() - make_interval(mins => 120)) RETURNING id`, task.ExplorationID).Scan(&intent)
	if err != nil {
		t.Fatal(err)
	}

	// no activity at all yet, but the intent is old and silent → stale
	ids, err := d.StaleRunningIntents(45, 10)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, id := range ids {
		if id == intent {
			found = true
		}
	}
	if !found {
		t.Fatal("old quiet running intent should be stale")
	}

	// recent activity revives it: not stale anymore
	if _, err := d.Exec(`INSERT INTO activity(exploration_id, node_id, worker, kind, summary, created_at)
VALUES ($1,$2,'work#1','text','fresh progress', now())`, task.ExplorationID, intent); err != nil {
		t.Fatal(err)
	}
	ids, err = d.StaleRunningIntents(45, 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		if id == intent {
			t.Fatal("intent with recent activity must not be stale")
		}
	}
	// reset flips running -> open exactly once
	reset, err := d.ResetRunningIntentToOpen(intent)
	if err != nil || !reset {
		t.Fatalf("reset: %v err=%v", reset, err)
	}
	reset, err = d.ResetRunningIntentToOpen(intent)
	if err != nil || reset {
		t.Fatalf("double reset: %v err=%v", reset, err)
	}

	// exploration/task resolution helpers
	exp, err := d.ExplorationIDByIntent(intent)
	if err != nil || exp != task.ExplorationID {
		t.Fatalf("exploration: %d err=%v", exp, err)
	}
	tid, err := d.TaskIDByExploration(exp)
	if err != nil || tid == "" {
		t.Fatalf("task: %q err=%v", tid, err)
	}
}
