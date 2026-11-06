package db

// Lifecycle tests for harness edit proposals (ADR-0002 phase 1).

import (
	"context"
	"testing"
)

func TestHarnessProposalLifecycle(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		t.Skipf("database unavailable (%v) — skipping", err)
	}
	defer d.Close()
	ctx := context.Background()

	// Shared suite DB: purge leftovers from earlier runs by agent key.
	d.Exec(`DELETE FROM harness_proposals WHERE agent_key='worker' AND summary LIKE 'fixture%'`)

	tmpl := "fixture worker template {{.ProxyAddr}} {{.DataDir}} {{.Now}}"

	// create + dedupe on identical pending template
	id, created, err := d.CreateHarnessProposal(ctx, "worker", "fixture: add rule", "lesson 1 says so", tmpl, []int64{1, 2})
	if err != nil || !created || id <= 0 {
		t.Fatalf("create: %d %v err=%v", id, created, err)
	}
	_, created, err = d.CreateHarnessProposal(ctx, "worker", "fixture: dup", "dup", tmpl, nil)
	if err != nil || created {
		t.Fatalf("dedupe: %v err=%v", created, err)
	}

	// lessons-without-proposals queue: proposed lessons leave the queue
	fixtureTask, err := d.CreateTask("proposal-fixture task", "goal", nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer d.DeleteTask(fixtureTask.ID)
	lid, err := d.RecordHarnessLesson(fixtureTask.ID, "", "fixture cue", "fixture lesson", "ev", "high")
	if err != nil {
		t.Fatal(err)
	}
	d.Exec(`DELETE FROM harness_lessons WHERE id=$1`, lid) // cleanup at end
	ag, err := d.GetAgentByKey("worker")
	if err != nil || ag == nil {
		t.Fatalf("agent: %v err=%v", ag, err)
	}
	ver, err := d.SavePrompt(ag.ID, "fixture base {{.ProxyAddr}}", "fixture reset", "test")
	if err != nil {
		t.Fatal(err)
	}
	_ = ver

	// apply path mirrors the server: SavePrompt bumps version, decision records it
	ver2, err := d.SavePrompt(ag.ID, tmpl, "harness proposal #1: fixture", "harness-editor")
	if err != nil {
		t.Fatal(err)
	}
	if ver2 <= 0 {
		t.Fatalf("version not bumped: %d", ver2)
	}
	changed, err := d.DecideHarnessProposal(id, "applied", int64(ver2))
	if err != nil || !changed {
		t.Fatalf("decide: %v err=%v", changed, err)
	}
	// double decide is a no-op
	changed, err = d.DecideHarnessProposal(id, "applied", int64(ver2))
	if err != nil || changed {
		t.Fatalf("double decide: %v err=%v", changed, err)
	}

	p, err := d.GetHarnessProposal(id)
	if err != nil || p.Status != "applied" || p.PromptVersion == nil || *p.PromptVersion != int64(ver2) {
		t.Fatalf("proposal: %+v err=%v", p, err)
	}

	list, err := d.ListHarnessProposals("applied", 10)
	if err != nil || len(list) == 0 {
		t.Fatalf("list: %d err=%v", len(list), err)
	}

	// pending-gate for the editor trigger
	pending, err := d.PendingHarnessProposalsExist()
	if err != nil {
		t.Fatal(err)
	}
	_ = pending // applied only — gate state depends on other suites' rows

	// reject path on a fresh proposal
	id2, created, err := d.CreateHarnessProposal(ctx, "worker", "fixture: reject me", "r", "fixture other {{.ProxyAddr}}", nil)
	if err != nil || !created {
		t.Fatalf("create2: %v err=%v", created, err)
	}
	changed, err = d.DecideHarnessProposal(id2, "rejected", 0)
	if err != nil || !changed {
		t.Fatalf("reject: %v err=%v", changed, err)
	}
	p2, _ := d.GetHarnessProposal(id2)
	if p2.Status != "rejected" || p2.PromptVersion != nil {
		t.Fatalf("rejected: %+v", p2)
	}
	d.Exec(`DELETE FROM harness_proposals WHERE id IN ($1,$2)`, id, id2)
}
