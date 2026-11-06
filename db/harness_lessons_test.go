package db

// Lifecycle tests for harness lessons (ADR-0002): recording, domain matching,
// reflection claim dedupe, context assembly.

import (
	"context"
	"strings"
	"testing"
)

func TestHarnessLessonLifecycle(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		t.Skipf("database unavailable (%v) — skipping", err)
	}
	defer d.Close()
	ctx := context.Background()

	if err := d.EnsureLLMUsageTable(); err != nil {
		t.Fatal(err)
	}
	task, err := d.CreateTask("lesson-fixture task", "goal", nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer d.DeleteTask(task.ID)
	// The suite shares one database: purge leftovers from earlier runs.
	defer d.Exec(`DELETE FROM harness_reflections WHERE task_id=$1`, task.ID)
	d.Exec(`DELETE FROM harness_lessons WHERE cue IN ('when a probe parses responses','when measuring rate limits')`)
	if _, err := d.Exec(`INSERT INTO task_scope(task_id, kind, domain) VALUES ($1,'root_domain','lesson-example.com') ON CONFLICT DO NOTHING`, task.ID); err != nil {
		t.Fatal(err)
	}

	// domain tag from task_scope
	domains, err := d.TaskRootDomains(task.ID)
	if err != nil || len(domains) != 1 || domains[0] != "lesson-example.com" {
		t.Fatalf("domains: %v err=%v", domains, err)
	}

	// global + domain lessons; matching honors both, ordered by confidence
	if _, err := d.RecordHarnessLesson(task.ID, "", "when a probe parses responses", "validate inputs and handle empty results explicitly", "2 crashed probes", "high"); err != nil {
		t.Fatal(err)
	}
	if _, err := d.RecordHarnessLesson(task.ID, "lesson-example.com", "when measuring rate limits", "distinguish per-account from per-ip buckets", "429 after 3 tries for an existing account", "medium"); err != nil {
		t.Fatal(err)
	}
	matches, err := d.MatchingHarnessLessons([]string{"lesson-example.com"}, 10)
	if err != nil || len(matches) != 2 {
		t.Fatalf("matches: %d err=%v", len(matches), err)
	}
	if matches[0].Cue != "when a probe parses responses" {
		t.Fatalf("confidence order wrong: %+v", matches[0])
	}
	// a different domain sees only the global lesson
	other, _ := d.MatchingHarnessLessons([]string{"elsewhere.example"}, 10)
	if len(other) != 1 || other[0].Domain != "" {
		t.Fatalf("global-only match: %+v", other)
	}

	// reflection claim is idempotent per task
	reflID, created, err := d.CreateHarnessReflection(ctx, task.ID)
	if err != nil || !created || reflID <= 0 {
		t.Fatalf("claim: %d %v err=%v", reflID, created, err)
	}
	_, created, err = d.CreateHarnessReflection(ctx, task.ID)
	if err != nil || created {
		t.Fatalf("double claim: %v err=%v", created, err)
	}
	// conversation creation binds and flips to running exactly once
	conv, created, err := d.CreateReflectionConversation(ctx, reflID, task.ID)
	if err != nil || !created {
		t.Fatalf("conv: %v err=%v", created, err)
	}
	if conv.AgentKey != HarnessReflectorAgentKey {
		t.Fatalf("agent key: %s", conv.AgentKey)
	}
	_, created, err = d.CreateReflectionConversation(ctx, reflID, task.ID)
	if err != nil || created {
		t.Fatalf("double conv: %v err=%v", created, err)
	}
	byConv, err := d.ReflectionForConversation(ctx, conv.ID)
	if err != nil || byConv == nil || byConv.TaskID != task.ID || byConv.Status != "running" {
		t.Fatalf("by conversation: %+v err=%v", byConv, err)
	}

	// run context assembles
	rc, err := d.GetReflectionRunContext(ctx, task.ID)
	if err != nil || rc == nil {
		t.Fatalf("context: %v err=%v", rc, err)
	}

	// finish paths
	if err := d.FinishHarnessReflection(reflID, "completed"); err != nil {
		t.Fatal(err)
	}
	if byConv, _ = d.ReflectionForConversation(ctx, conv.ID); byConv.Status != "completed" {
		t.Fatalf("status: %+v", byConv)
	}

	// list API shape
	list, err := d.ListHarnessLessons(10)
	if err != nil || len(list) != 2 {
		t.Fatalf("list: %d err=%v", len(list), err)
	}
	if !strings.Contains(list[0].Lesson, "per-account") && !strings.Contains(list[1].Lesson, "per-account") {
		t.Fatalf("lesson content lost: %+v", list)
	}
}
