package db

import "time"

// ProvenanceCleanup is one audit record of an operator clearing a task's artifact
// directory. The row is the durable trace that the files were removed on purpose and
// how much was reclaimed, so a later reader can tell "cleaned up" from "lost".
type ProvenanceCleanup struct {
	ID         int64     `json:"id"`
	TaskID     int64     `json:"task_id"`
	At         time.Time `json:"at"`
	FreedBytes int64     `json:"freed_bytes"`
	FreedFiles int64     `json:"freed_files"`
	Actor      string    `json:"actor"`
	Note       string    `json:"note"`
}

// SaveProvenanceCleanup appends one cleanup audit row.
func (d *DB) SaveProvenanceCleanup(taskID, freedBytes, freedFiles int64, actor, note string) error {
	_, err := d.Exec(`
INSERT INTO task_provenance_cleanups(task_id, freed_bytes, freed_files, actor, note)
VALUES ($1, $2, $3, $4, $5)`, taskID, freedBytes, freedFiles, actor, note)
	return err
}

// ListProvenanceCleanups returns a task's cleanup history, newest first.
func (d *DB) ListProvenanceCleanups(taskID int64) ([]ProvenanceCleanup, error) {
	rows, err := d.Query(`
SELECT id, task_id, at, freed_bytes, freed_files, actor, note
FROM task_provenance_cleanups WHERE task_id=$1 ORDER BY at DESC, id DESC`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ProvenanceCleanup
	for rows.Next() {
		var c ProvenanceCleanup
		if err := rows.Scan(&c.ID, &c.TaskID, &c.At, &c.FreedBytes, &c.FreedFiles, &c.Actor, &c.Note); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
