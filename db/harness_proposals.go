package db

// Harness edit proposals (phase 1 of auto harness refinement, ADR-0002). The
// harness editor agent turns adopted lessons into a concrete prompt-template
// proposal; the operator applies/rejects it in the UI. Apply bumps the agent
// prompt version through SavePrompt, so roll-back is the prompt versions page.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

const HarnessEditorAgentKey = "harness-editor"

// HarnessProposal is one proposed prompt edit.
type HarnessProposal struct {
	ID            int64           `json:"id"`
	AgentKey      string          `json:"agent_key"`
	Summary       string          `json:"summary"`
	Rationale     string          `json:"rationale"`
	Template      string          `json:"template"`
	LessonIDs     json.RawMessage `json:"lesson_ids"`
	Status        string          `json:"status"`
	PromptVersion *int64          `json:"prompt_version"`
	CreatedAt     time.Time       `json:"created_at"`
	DecidedAt     *time.Time      `json:"decided_at"`
}

const harnessProposalCols = `id, agent_key, summary, rationale, template, lesson_ids, status, prompt_version, created_at, decided_at`

func scanProposal(row interface{ Scan(...any) error }) (*HarnessProposal, error) {
	var p HarnessProposal
	var ver *int64
	if err := row.Scan(&p.ID, &p.AgentKey, &p.Summary, &p.Rationale, &p.Template, &p.LessonIDs, &p.Status, &ver, &p.CreatedAt, &p.DecidedAt); err != nil {
		return nil, err
	}
	p.PromptVersion = ver
	return &p, nil
}

// CreateHarnessProposal stores one proposal. A pending proposal with the same
// agent_key+template already in flight is a no-op (created=false) so a
// re-triggered editor cannot pile up duplicates.
func (d *DB) CreateHarnessProposal(ctx context.Context, agentKey, summary, rationale, template string, lessonIDs []int64) (int64, bool, error) {
	ids, _ := json.Marshal(lessonIDs)
	if len(lessonIDs) == 0 {
		ids = json.RawMessage("[]")
	}
	var id int64
	err := d.QueryRowContext(ctx, `
INSERT INTO harness_proposals(agent_key, summary, rationale, template, lesson_ids)
SELECT $1,$2,$3,$4,$5
WHERE NOT EXISTS (
	SELECT 1 FROM harness_proposals
	WHERE status='pending' AND agent_key=$1 AND template=$4)
RETURNING id`, agentKey, summary, rationale, template, ids).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return id, true, nil
}

// ListHarnessProposals returns proposals by status (empty = all), newest first.
func (d *DB) ListHarnessProposals(status string, limit int) ([]HarnessProposal, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	q := `SELECT ` + harnessProposalCols + ` FROM harness_proposals ORDER BY id DESC LIMIT $1`
	args := []any{limit}
	if status != "" {
		q = `SELECT ` + harnessProposalCols + ` FROM harness_proposals WHERE status=$1 ORDER BY id DESC LIMIT $2`
		args = []any{status, limit}
	}
	rows, err := d.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []HarnessProposal
	for rows.Next() {
		p, err := scanProposal(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

// GetHarnessProposal loads one proposal for deciding.
func (d *DB) GetHarnessProposal(id int64) (*HarnessProposal, error) {
	return scanProposal(d.QueryRow(`SELECT `+harnessProposalCols+` FROM harness_proposals WHERE id=$1`, id))
}

// DecideHarnessProposal flips a pending proposal to applied/rejected. The
// pending guard makes double-clicks harmless.
func (d *DB) DecideHarnessProposal(id int64, status string, promptVersion int64) (bool, error) {
	var ver any
	if promptVersion > 0 {
		ver = promptVersion
	}
	res, err := d.Exec(`UPDATE harness_proposals SET status=$2, prompt_version=$3, decided_at=now()
WHERE id=$1 AND status='pending'`, id, status, ver)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// LessonsWithoutProposals returns adopted lessons (min confidence) that no
// pending/applied proposal references — the editor's work queue.
func (d *DB) LessonsWithoutProposals(minConfidence string, limit int) ([]HarnessLesson, error) {
	if limit <= 0 || limit > 20 {
		limit = 10
	}
	rank := map[string]int{"high": 0, "medium": 1}
	minRank, ok := rank[minConfidence]
	if !ok {
		minRank = 1
	}
	rows, err := d.Query(`SELECT `+harnessLessonCols+` FROM harness_lessons l
WHERE CASE l.confidence WHEN 'high' THEN 0 WHEN 'medium' THEN 1 ELSE 2 END <= $1
  AND NOT EXISTS (
    SELECT 1 FROM harness_proposals p
    WHERE p.lesson_ids @> to_jsonb(l.id) AND p.status IN ('pending','applied'))
ORDER BY CASE l.confidence WHEN 'high' THEN 0 ELSE 1 END, l.id DESC
LIMIT $2`, minRank, limit)
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

// PendingHarnessProposalsExist gates the editor trigger (one proposal batch
// at a time — the operator decides before the next round).
func (d *DB) PendingHarnessProposalsExist() (bool, error) {
	var n int
	if err := d.QueryRow(`SELECT count(*) FROM harness_proposals WHERE status='pending'`).Scan(&n); err != nil {
		return false, err
	}
	return n > 0, nil
}
