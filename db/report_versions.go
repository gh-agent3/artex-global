package db

import "time"

// TaskReportVersion is one stored revision of a task's penetration-test report.
// The first row is the generated report (version 1.0); every chat-driven revision
// adds a child version (1.1, 1.2, ...) and keeps the previous one readable, so the
// operator can compare and roll back by simply selecting another node.
type TaskReportVersion struct {
	ID        int64     `json:"id"`
	TaskID    int64     `json:"task_id"`
	Version   string    `json:"version"`
	Content   string    `json:"content,omitempty"`
	Note      string    `json:"note"`
	ParentID  *int64    `json:"parent_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	Bytes     int       `json:"bytes"`
}

// ListReportVersions returns a task's report versions, newest first, without content.
func (d *DB) ListReportVersions(taskID int64) ([]TaskReportVersion, error) {
	rows, err := d.Query(`
SELECT id, task_id, version, note, parent_id, created_at, length(content)
FROM task_report_versions WHERE task_id=$1 ORDER BY created_at DESC, id DESC`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TaskReportVersion{}
	for rows.Next() {
		var v TaskReportVersion
		if err := rows.Scan(&v.ID, &v.TaskID, &v.Version, &v.Note, &v.ParentID, &v.CreatedAt, &v.Bytes); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// GetReportVersion returns one version including its Markdown.
func (d *DB) GetReportVersion(taskID, id int64) (*TaskReportVersion, error) {
	var v TaskReportVersion
	err := d.QueryRow(`
SELECT id, task_id, version, content, note, parent_id, created_at, length(content)
FROM task_report_versions WHERE task_id=$1 AND id=$2`, taskID, id).
		Scan(&v.ID, &v.TaskID, &v.Version, &v.Content, &v.Note, &v.ParentID, &v.CreatedAt, &v.Bytes)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// SaveReportVersion stores one revision and returns its id.
func (d *DB) SaveReportVersion(taskID int64, version, content, note string, parentID *int64, profileID int64) (int64, error) {
	var id int64
	var prof any
	if profileID > 0 {
		prof = profileID
	}
	err := d.QueryRow(`
INSERT INTO task_report_versions(task_id, version, content, note, parent_id, profile_id)
VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`, taskID, version, content, note, parentID, prof).Scan(&id)
	return id, err
}

// LatestReportVersion returns the newest version of a task's report, if any.
func (d *DB) LatestReportVersion(taskID int64) (*TaskReportVersion, error) {
	var v TaskReportVersion
	err := d.QueryRow(`
SELECT id, task_id, version, content, note, parent_id, created_at, length(content)
FROM task_report_versions WHERE task_id=$1 ORDER BY created_at DESC, id DESC LIMIT 1`, taskID).
		Scan(&v.ID, &v.TaskID, &v.Version, &v.Content, &v.Note, &v.ParentID, &v.CreatedAt, &v.Bytes)
	if err != nil {
		return nil, err
	}
	return &v, nil
}
