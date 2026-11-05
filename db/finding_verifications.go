package db

// Independent verification of findings (P2 verifier). Unlike finding_retests
// (operator-triggered rechecks), verifications are enqueued automatically by
// severity policy when a finding is recorded: critical/high always, medium by
// a deterministic sample, low never. The verifier is a separate conversation
// agent with read-only tools; its verdict is mirrored onto findings.validation.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const FindingVerifierAgentKey = "verifier"

// FindingVerification is one verification run. Immutable once finished.
type FindingVerification struct {
	ID             int64      `json:"id"`
	FindingID      int64      `json:"finding_id"`
	ConversationID *int64     `json:"conversation_id"`
	Status         string     `json:"status"`
	Reason         string     `json:"reason"`
	Notes          string     `json:"notes"`
	CreatedAt      time.Time  `json:"created_at"`
	StartedAt      *time.Time `json:"started_at"`
	FinishedAt     *time.Time `json:"finished_at"`
}

const verificationCols = `id, finding_id, conversation_id, status, reason, notes, created_at, started_at, finished_at`

// FindingPendingVerificationRow is one finding the scanner judged in scope.
type FindingPendingVerificationRow struct {
	ID       int64  `json:"id"`
	Severity string `json:"severity"`
}

// FindingsPendingVerification lists findings that have no verification row at
// all yet (fresh recordings), newest first. Scanner applies the severity policy.
func (d *DB) FindingsPendingVerification(ctx context.Context, limit int) ([]FindingPendingVerificationRow, error) {
	rows, err := d.QueryContext(ctx, `
SELECT f.id, f.severity FROM findings f
WHERE NOT EXISTS (SELECT 1 FROM finding_verifications v WHERE v.finding_id = f.id)
ORDER BY f.id DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FindingPendingVerificationRow
	for rows.Next() {
		var r FindingPendingVerificationRow
		if err := rows.Scan(&r.ID, &r.Severity); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// CreateFindingVerification enqueues one verification and its verifier
// conversation in a single transaction. The snapshot (finding + assets +
// constraints) is captured now so the agent works from a frozen input.
// (created=false) when the finding already has an active verification.
func (d *DB) CreateFindingVerification(ctx context.Context, findingID int64, notes string) (*FindingVerification, *Conversation, bool, error) {
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, false, err
	}
	defer tx.Rollback()
	var title string
	var snapshot []byte
	err = tx.QueryRowContext(ctx, `SELECT COALESCE(NULLIF(f.name,''), NULLIF(f.vulnclass,''), 'uncategorized'),
	jsonb_build_object('finding', to_jsonb(f),
	 'assets', COALESCE((SELECT jsonb_agg(to_jsonb(a)) FROM assets a WHERE f.asset_ids @> to_jsonb(ARRAY[a.id])), '[]'::jsonb),
	 'constraints', COALESCE((SELECT jsonb_agg(to_jsonb(c)) FROM task_constraints c JOIN tasks t ON t.exploration_id=c.exploration_id WHERE t.id=f.task_id), '[]'::jsonb))
	FROM findings f WHERE f.id=$1 FOR UPDATE OF f`, findingID).Scan(&title, &snapshot)
	if err != nil {
		return nil, nil, false, err
	}
	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM finding_verifications WHERE finding_id=$1 AND status IN ('pending','running','verified','refuted','inconclusive'))`, findingID).Scan(&exists); err != nil {
		return nil, nil, false, err
	}
	if exists {
		return nil, nil, false, nil
	}
	if runes := []rune(title); len(runes) > 100 {
		title = string(runes[:100])
	}
	c, err := scanConv(tx.QueryRowContext(ctx, `INSERT INTO conversations(agent_key,title) VALUES ($1,$2) RETURNING `+convCols,
		FindingVerifierAgentKey, fmt.Sprintf("verify #%d · %s", findingID, title)))
	if err != nil {
		return nil, nil, false, err
	}
	var v FindingVerification
	var conv *int64
	err = tx.QueryRowContext(ctx, `INSERT INTO finding_verifications(finding_id,conversation_id,notes,snapshot) VALUES ($1,$2,$3,$4) RETURNING `+verificationCols,
		findingID, c.ID, strings.TrimSpace(notes), snapshot).Scan(&v.ID, &v.FindingID, &conv, &v.Status, &v.Reason, &v.Notes, &v.CreatedAt, &v.StartedAt, &v.FinishedAt)
	if err != nil {
		return nil, nil, false, err
	}
	v.ConversationID = conv
	if _, err = tx.ExecContext(ctx, `INSERT INTO conversation_activities(conversation_id,worker,kind,summary,detail) VALUES ($1,$2,'user',$3,$4)`,
		c.ID, FindingVerifierAgentKey, fmt.Sprintf("independently verify finding #%d", findingID), VerificationInitialMessage(findingID)); err != nil {
		return nil, nil, false, err
	}
	if err = tx.Commit(); err != nil {
		return nil, nil, false, err
	}
	return &v, &c, true, nil
}

// VerificationInitialMessage is the verifier turn text, seeded with the
// conversation and replayed by the runner.
func VerificationInitialMessage(findingID int64) string {
	return fmt.Sprintf(`Independently verify finding #%d. Call get_finding_verify_context for the frozen snapshot (finding, evidence, assets, constraints). Judge whether the recorded claim is proven by the captured evidence: verified / refuted / inconclusive, then call record_finding_verification_result with the verdict and a reason. Read-only: do not probe the target, do not change the finding, do not create tasks.`, findingID)
}

// FinishFindingVerification closes a row with its verdict and mirrors the
// outcome onto findings.validation (verified / refuted / unverified).
func (d *DB) FinishFindingVerification(id int64, status, reason string) error {
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var findingID int64
	if err := tx.QueryRow(`UPDATE finding_verifications SET status=$2, reason=$3, finished_at=now() WHERE id=$1 RETURNING finding_id`,
		id, status, reason).Scan(&findingID); err != nil {
		return err
	}
	validation := "unverified"
	switch status {
	case "verified":
		validation = "verified"
	case "refuted":
		validation = "refuted"
	}
	if _, err := tx.Exec(`UPDATE findings SET validation=$2 WHERE id=$1`, findingID, validation); err != nil {
		return err
	}
	return tx.Commit()
}

// ListFindingVerifications returns the history for one finding, newest first.
func (d *DB) ListFindingVerifications(findingID int64) ([]FindingVerification, error) {
	rows, err := d.Query(`SELECT `+verificationCols+` FROM finding_verifications WHERE finding_id=$1 ORDER BY id DESC`, findingID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FindingVerification
	for rows.Next() {
		var v FindingVerification
		var conv *int64
		if err := rows.Scan(&v.ID, &v.FindingID, &conv, &v.Status, &v.Reason, &v.Notes, &v.CreatedAt, &v.StartedAt, &v.FinishedAt); err != nil {
			return nil, err
		}
		v.ConversationID = conv
		out = append(out, v)
	}
	return out, rows.Err()
}

// VerificationForConversation returns the verification row bound to a
// conversation, or nil. Used by the context tool.
func (d *DB) VerificationForConversation(ctx context.Context, convID int64) (*FindingVerification, error) {
	var v FindingVerification
	var conv *int64
	err := d.QueryRowContext(ctx, `SELECT `+verificationCols+` FROM finding_verifications WHERE conversation_id=$1`, convID).
		Scan(&v.ID, &v.FindingID, &conv, &v.Status, &v.Reason, &v.Notes, &v.CreatedAt, &v.StartedAt, &v.FinishedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	v.ConversationID = conv
	return &v, nil
}

// VerificationSnapshot returns the frozen snapshot captured at enqueue time.
func (d *DB) VerificationSnapshot(id int64) (json.RawMessage, error) {
	var snap json.RawMessage
	err := d.QueryRow(`SELECT snapshot FROM finding_verifications WHERE id=$1`, id).Scan(&snap)
	return snap, err
}
