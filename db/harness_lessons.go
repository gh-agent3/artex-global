package db

// Harness lessons (harnessing-level recursion, inspired by Google Harness²'s
// experience library). A reflector agent derives at most one durable lesson
// from a finished task: cue (the SHAPE of situation — a transferable pattern),
// lesson (the operation, naming what it concerns), evidence (concrete
// observations from the run), confidence. New tasks inject matching lessons
// into planner/worker prompts. One reflection per task (harness_reflections).

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"time"
)

const HarnessReflectorAgentKey = "reflector"

// HarnessLesson is one adopted lesson. Immutable once written.
type HarnessLesson struct {
	ID         int64     `json:"id"`
	TaskID     *int64    `json:"task_id"`
	Domain     string    `json:"domain"`
	Cue        string    `json:"cue"`
	Lesson     string    `json:"lesson"`
	Evidence   string    `json:"evidence"`
	Confidence string    `json:"confidence"`
	CreatedAt  time.Time `json:"created_at"`
}

const harnessLessonCols = `id, task_id, domain, cue, lesson, evidence, confidence, created_at`

// TaskRootDomains returns the root_domain scope entries of one task, used to
// domain-tag lessons and to match lessons for new tasks.
func (d *DB) TaskRootDomains(taskID int64) ([]string, error) {
	rows, err := d.Query(`SELECT domain FROM task_scope WHERE task_id=$1 AND kind='root_domain' AND domain IS NOT NULL AND domain <> ''`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// RecordHarnessLesson inserts one lesson. Cue/lesson/evidence must be
// non-empty (the tool enforces this; the row stores what it gets).
func (d *DB) RecordHarnessLesson(taskID int64, domain, cue, lesson, evidence, confidence string) (int64, error) {
	var id int64
	err := d.QueryRow(`INSERT INTO harness_lessons(task_id, domain, cue, lesson, evidence, confidence)
VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`,
		taskID, domain, cue, lesson, evidence, confidence).Scan(&id)
	return id, err
}

// HarnessLessonMatch is a lesson with the domain it matched through ("" when
// global), for prompt rendering.
type HarnessLessonMatch struct {
	HarnessLesson
	MatchedDomain string `json:"-"`
}

// MatchingHarnessLessons returns up to limit lessons relevant to a task:
// global lessons (domain=”) plus lessons whose domain equals one of the
// task's root_domain scopes. Ordered by confidence, then recency.
func (d *DB) MatchingHarnessLessons(domains []string, limit int) ([]HarnessLessonMatch, error) {
	if limit <= 0 {
		limit = 6
	}
	args := []any{}
	domainSet := "''"
	for _, dom := range domains {
		args = append(args, dom)
		domainSet += ",$" + strconv.Itoa(len(args))
	}
	args = append(args, limit)
	rows, err := d.Query(`
SELECT `+harnessLessonCols+`
FROM harness_lessons l
WHERE l.domain = '' OR l.domain IN (`+domainSet+`)
ORDER BY CASE l.confidence WHEN 'high' THEN 0 WHEN 'medium' THEN 1 ELSE 2 END, l.id DESC
LIMIT $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []HarnessLessonMatch
	for rows.Next() {
		var l HarnessLesson
		var taskID *int64
		if err := rows.Scan(&l.ID, &taskID, &l.Domain, &l.Cue, &l.Lesson, &l.Evidence, &l.Confidence, &l.CreatedAt); err != nil {
			return nil, err
		}
		l.TaskID = taskID
		out = append(out, HarnessLessonMatch{HarnessLesson: l, MatchedDomain: l.Domain})
	}
	return out, rows.Err()
}

// ListHarnessLessons returns recent lessons for the API (newest first).
func (d *DB) ListHarnessLessons(limit int) ([]HarnessLesson, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := d.Query(`SELECT `+harnessLessonCols+` FROM harness_lessons ORDER BY id DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []HarnessLesson
	for rows.Next() {
		var l HarnessLesson
		var taskID *int64
		if err := rows.Scan(&l.ID, &taskID, &l.Domain, &l.Cue, &l.Lesson, &l.Evidence, &l.Confidence, &l.CreatedAt); err != nil {
			return nil, err
		}
		l.TaskID = taskID
		out = append(out, l)
	}
	return out, rows.Err()
}

// ── reflections (one per finished task) ─────────────────────────────────────

// HarnessReflection is the bookkeeping row for one reflector run.
type HarnessReflection struct {
	ID             int64      `json:"id"`
	TaskID         int64      `json:"task_id"`
	ConversationID *int64     `json:"conversation_id"`
	Status         string     `json:"status"`
	CreatedAt      time.Time  `json:"created_at"`
	StartedAt      *time.Time `json:"started_at"`
	FinishedAt     *time.Time `json:"finished_at"`
}

const reflectionCols = `id, task_id, conversation_id, status, created_at, started_at, finished_at`

// TasksPendingReflection lists terminal tasks finished at least minAge ago
// that have no reflection row yet. Scanner applies a policy floor on size.
func (d *DB) TasksPendingReflection(ctx context.Context, minAgeMinutes, activityFloor, limit int) ([]int64, error) {
	cutoff := time.Now().Add(-time.Duration(minAgeMinutes) * time.Minute)
	rows, err := d.QueryContext(ctx, `
SELECT t.id FROM tasks t
WHERE t.status IN ('done','failed','timeout')
  AND t.completed_at IS NOT NULL
  AND t.completed_at < $1
  AND (SELECT count(*) FROM activity a JOIN exploration_nodes n ON n.id = a.node_id
       WHERE n.exploration_id = t.exploration_id) >= $2
  AND NOT EXISTS (SELECT 1 FROM harness_reflections r WHERE r.task_id = t.id)
ORDER BY t.completed_at LIMIT $3`, cutoff, activityFloor, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// CreateHarnessReflection inserts the pending row and returns its id;
// (created=false) when the task already has one (unique constraint —
// concurrent scanners dedupe).
func (d *DB) CreateHarnessReflection(ctx context.Context, taskID int64) (int64, bool, error) {
	var id int64
	err := d.QueryRowContext(ctx, `INSERT INTO harness_reflections(task_id) VALUES ($1) ON CONFLICT (task_id) DO NOTHING RETURNING id`, taskID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return id, true, nil
}

// CreateReflectionConversation creates the reflector conversation for a claimed
// reflection in one transaction and marks it running. (created=false) when the
// reflection is no longer pending (restart/duplicate guard).
func (d *DB) CreateReflectionConversation(ctx context.Context, reflectionID, taskID int64) (*Conversation, bool, error) {
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback()
	var status string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM harness_reflections WHERE id=$1 FOR UPDATE`, reflectionID).Scan(&status); err != nil {
		return nil, false, err
	}
	if status != "pending" {
		return nil, false, nil
	}
	c, err := scanConv(tx.QueryRowContext(ctx, `INSERT INTO conversations(agent_key,title) VALUES ($1,$2) RETURNING `+convCols,
		HarnessReflectorAgentKey, "reflect on task #"+strconv.FormatInt(taskID, 10)))
	if err != nil {
		return nil, false, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE harness_reflections SET status='running', conversation_id=$2, started_at=now() WHERE id=$1`, reflectionID, c.ID); err != nil {
		return nil, false, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO conversation_activities(conversation_id,worker,kind,summary,detail) VALUES ($1,$2,'user',$3,$4)`,
		c.ID, HarnessReflectorAgentKey, "reflect on task #"+strconv.FormatInt(taskID, 10), ReflectionInitialMessageFor(taskID)); err != nil {
		return nil, false, err
	}
	if err := tx.Commit(); err != nil {
		return nil, false, err
	}
	return &c, true, nil
}

// ReflectionInitialMessageFor is the reflector turn text, seeded with the
// conversation and replayed by the runner.
func ReflectionInitialMessageFor(taskID int64) string {
	return "Review the finished task #" + strconv.FormatInt(taskID, 10) + " and extract at most one durable lesson. " +
		"Start with get_reflection_context. If nothing durable is visible, end without calling record_harness_lesson."
}

// MarkReflectionRunning binds the conversation and flips status.
func (d *DB) MarkReflectionRunning(ctx context.Context, id, convID int64) error {
	_, err := d.ExecContext(ctx, `UPDATE harness_reflections SET status='running', conversation_id=$2, started_at=now() WHERE id=$1`, id, convID)
	return err
}

// FinishHarnessReflection closes the row. outcome is completed | no_entry | error.
func (d *DB) FinishHarnessReflection(id int64, outcome string) error {
	_, err := d.Exec(`UPDATE harness_reflections SET status=$2, finished_at=now() WHERE id=$1`, id, outcome)
	return err
}

// ReflectionForConversation returns the reflection bound to a conversation.
func (d *DB) ReflectionForConversation(ctx context.Context, convID int64) (*HarnessReflection, error) {
	var r HarnessReflection
	var conv *int64
	err := d.QueryRowContext(ctx, `SELECT `+reflectionCols+` FROM harness_reflections WHERE conversation_id=$1`, convID).
		Scan(&r.ID, &r.TaskID, &conv, &r.Status, &r.CreatedAt, &r.StartedAt, &r.FinishedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	r.ConversationID = conv
	return &r, nil
}

// ReflectionRunContext is the frozen input handed to the reflector agent.
type ReflectionRunContext struct {
	Task        any   `json:"task"`
	Stats       any   `json:"stats"`
	Refuted     any   `json:"refuted_findings"`
	LessonCount int64 `json:"prior_lessons"`
}

// GetReflectionRunContext assembles the read-only context for one task.
func (d *DB) GetReflectionRunContext(ctx context.Context, taskID int64) (*ReflectionRunContext, error) {
	task, err := d.GetTask(taskID)
	if err != nil || task == nil {
		return nil, err
	}
	var stats struct {
		Activity      int64 `json:"activity"`
		ToolErrors    int64 `json:"tool_errors"`
		LLMCalls      int64 `json:"llm_calls"`
		LLMErrors     int64 `json:"llm_errors"`
		Findings      int64 `json:"findings"`
		Verified      int64 `json:"verified"`
		Refuted       int64 `json:"refuted"`
		Verifications int64 `json:"verifications"`
	}
	err = d.QueryRowContext(ctx, `
SELECT
 (SELECT count(*) FROM activity a JOIN exploration_nodes n ON n.id=a.node_id WHERE n.exploration_id=$1),
 (SELECT count(*) FROM activity a JOIN exploration_nodes n ON n.id=a.node_id WHERE n.exploration_id=$1 AND a.is_error),
 (SELECT count(*) FROM llm_usage WHERE task_id=$3),
 (SELECT count(*) FROM llm_usage WHERE task_id=$3 AND status='error'),
 (SELECT count(*) FROM findings WHERE task_id=$2),
 (SELECT count(*) FROM findings WHERE task_id=$2 AND validation='verified'),
 (SELECT count(*) FROM findings WHERE task_id=$2 AND validation='refuted'),
 (SELECT count(*) FROM finding_verifications v JOIN findings f ON f.id=v.finding_id WHERE f.task_id=$2)`,
		task.ExplorationID, taskID, strconv.FormatInt(taskID, 10)).Scan(&stats.Activity, &stats.ToolErrors, &stats.LLMCalls, &stats.LLMErrors,
		&stats.Findings, &stats.Verified, &stats.Refuted, &stats.Verifications)
	if err != nil {
		return nil, err
	}
	rows, err := d.QueryContext(ctx, `SELECT f.id, f.name, f.severity, v.reason FROM findings f
JOIN finding_verifications v ON v.finding_id = f.id AND v.verdict = 'refuted'
WHERE f.task_id=$1 ORDER BY f.id DESC LIMIT 5`, taskID)
	if err != nil {
		return nil, err
	}
	var refuted []map[string]any
	for rows.Next() {
		var id int64
		var name, sev, reason string
		if err := rows.Scan(&id, &name, &sev, &reason); err != nil {
			rows.Close()
			return nil, err
		}
		refuted = append(refuted, map[string]any{"id": id, "name": name, "severity": sev, "reason": reason})
	}
	rows.Close()
	var n int64
	if err := d.QueryRowContext(ctx, `SELECT count(*) FROM harness_lessons`).Scan(&n); err != nil {
		return nil, err
	}
	return &ReflectionRunContext{Task: task, Stats: stats, Refuted: refuted, LessonCount: n}, nil
}

// TerminalTasksOlderThan lists terminal tasks completed more than days ago —
// the diskguard's rotation candidate set.
func (d *DB) TerminalTasksOlderThan(days, limit int) ([]int64, error) {
	cutoff := time.Now().AddDate(0, 0, -days)
	rows, err := d.Query(`SELECT id FROM tasks
WHERE status IN ('done','failed','timeout','archived')
  AND completed_at IS NOT NULL AND completed_at < $1
ORDER BY completed_at LIMIT $2`, cutoff, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// ── intent watchdog (worker-liveness backstop) ───────────────────────────────

// StaleRunningIntents finds intents stuck in 'running' whose last activity is
// older than maxIdleMinutes — a worker that died silently (observed in the
// wild: a stalled stream froze the planner loop, and with it the worker
// supervision path, leaving the intent 'running' forever). updated_at is
// trigger-managed (always now() on touch) so it cannot age; created_at is
// immutable and bounds false positives on freshly claimed intents.
func (d *DB) StaleRunningIntents(maxIdleMinutes, limit int) ([]int64, error) {
	rows, err := d.Query(`
SELECT n.id FROM exploration_nodes n
WHERE n.kind='intent' AND n.state='running'
  AND n.created_at < now() - make_interval(mins => $1)
  AND NOT EXISTS (
    SELECT 1 FROM activity a
    WHERE a.node_id = n.id AND a.created_at > now() - make_interval(mins => $1))
ORDER BY n.id LIMIT $2`, maxIdleMinutes, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// ResetRunningIntentToOpen atomically returns a stale running intent to open;
// (reset=false) when it is no longer running. The engine's worker loop
// re-claims open intents on its next poll.
func (d *DB) ResetRunningIntentToOpen(id int64) (bool, error) {
	res, err := d.Exec(`UPDATE exploration_nodes SET state='open' WHERE id=$1 AND kind='intent' AND state='running'`, id)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// TaskIDByExploration resolves a task id for notifying after a reset.
func (d *DB) TaskIDByExploration(expID int64) (string, error) {
	var id int64
	err := d.QueryRow(`SELECT id FROM tasks WHERE exploration_id=$1`, expID).Scan(&id)
	return strconv.FormatInt(id, 10), err
}

// ExplorationIDByIntent resolves the exploration an intent belongs to.
func (d *DB) ExplorationIDByIntent(id int64) (int64, error) {
	var exp int64
	err := d.QueryRow(`SELECT exploration_id FROM exploration_nodes WHERE id=$1`, id).Scan(&exp)
	return exp, err
}
