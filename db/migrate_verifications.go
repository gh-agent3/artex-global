package db

// migrateFindingVerificationsShape upgrades the finding_verifications table from
// the first deployed shape (status carried the verdict:
// pending/running/verified/refuted/inconclusive/error) to the final shape
// (status is the lifecycle pending/running/completed/failed/stopped, verdict is
// a separate column). Idempotent: a table that already has the verdict column
// is left untouched. Data is preserved — finished rows map their old status
// into the verdict column and become 'completed'.

import (
	"context"
	"database/sql"
)

func migrateFindingVerificationsShape(ctx context.Context, conn *sql.Conn) error {
	var hasVerdict bool
	err := conn.QueryRowContext(ctx, `
SELECT EXISTS (
	SELECT 1 FROM information_schema.columns
	WHERE table_name='finding_verifications' AND column_name='verdict'
)`).Scan(&hasVerdict)
	if err != nil {
		return err
	}
	if hasVerdict {
		return nil
	}
	stmts := []string{
		`ALTER TABLE finding_verifications RENAME TO finding_verifications_old`,
		// Same DDL as schema.sql (kept inline: CREATE TABLE IF NOT EXISTS in
		// schema.sql is a no-op while the old table exists under its old name).
		`CREATE TABLE finding_verifications (
    id BIGSERIAL PRIMARY KEY,
    finding_id BIGINT NOT NULL REFERENCES findings(id) ON DELETE CASCADE,
    conversation_id BIGINT UNIQUE REFERENCES conversations(id) ON DELETE SET NULL,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','running','completed','failed','stopped')),
    verdict TEXT NOT NULL DEFAULT '' CHECK (verdict IN ('','verified','refuted','inconclusive')),
    notes TEXT NOT NULL DEFAULT '',
    reason TEXT NOT NULL DEFAULT '',
    snapshot JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ
)`,
		`INSERT INTO finding_verifications(id, finding_id, conversation_id, status, verdict, notes, reason, snapshot, created_at, started_at, finished_at)
SELECT id, finding_id, conversation_id,
	CASE WHEN status IN ('verified','refuted','inconclusive') THEN 'completed' ELSE status END,
	CASE WHEN status IN ('verified','refuted','inconclusive') THEN status ELSE '' END,
	notes, reason, snapshot, created_at, started_at, finished_at
FROM finding_verifications_old`,
		`DROP TABLE finding_verifications_old`,
	}
	for _, q := range stmts {
		if _, err := conn.ExecContext(ctx, q); err != nil {
			return err
		}
	}
	return nil
}
