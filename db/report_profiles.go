package db

import "time"

// ReportProfile is one report FORMAT: the section skeleton and the writing rules the
// report writer must follow. The built-in profile reproduces the standard deterministic
// report; a custom profile is applied as a rewrite instruction over that canonical text,
// so the facts always come from the graph and only the shape changes.
type ReportProfile struct {
	ID           int64     `json:"id"`
	Name         string    `json:"name"`
	Instructions string    `json:"instructions"`
	Builtin      bool      `json:"builtin"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// ListReportProfiles returns every profile, built-in first.
func (d *DB) ListReportProfiles() ([]ReportProfile, error) {
	rows, err := d.Query(`
SELECT id, name, instructions, builtin, created_at, updated_at
FROM report_profiles ORDER BY builtin DESC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ReportProfile{}
	for rows.Next() {
		var p ReportProfile
		if err := rows.Scan(&p.ID, &p.Name, &p.Instructions, &p.Builtin, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// GetReportProfile returns one profile.
func (d *DB) GetReportProfile(id int64) (*ReportProfile, error) {
	var p ReportProfile
	err := d.QueryRow(`
SELECT id, name, instructions, builtin, created_at, updated_at
FROM report_profiles WHERE id=$1`, id).
		Scan(&p.ID, &p.Name, &p.Instructions, &p.Builtin, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// SaveReportProfile inserts (id=0) or updates a profile.
func (d *DB) SaveReportProfile(id int64, name, instructions string) (int64, error) {
	if id > 0 {
		_, err := d.Exec(`
UPDATE report_profiles SET name=$2, instructions=$3, updated_at=now() WHERE id=$1`, id, name, instructions)
		return id, err
	}
	var newID int64
	err := d.QueryRow(`
INSERT INTO report_profiles(name, instructions) VALUES ($1,$2) RETURNING id`, name, instructions).Scan(&newID)
	return newID, err
}

// DeleteReportProfile removes a custom profile (the built-in one is never deleted).
func (d *DB) DeleteReportProfile(id int64) error {
	_, err := d.Exec(`DELETE FROM report_profiles WHERE id=$1 AND NOT builtin`, id)
	return err
}

// TaskReportProfile returns the profile selected for a task, or 0 when none is set.
func (d *DB) TaskReportProfile(taskID int64) int64 {
	var id int64
	_ = d.QueryRow(`SELECT profile_id FROM task_report_settings WHERE task_id=$1`, taskID).Scan(&id)
	return id
}

// SetTaskReportProfile stores the profile selected for a task.
func (d *DB) SetTaskReportProfile(taskID, profileID int64) error {
	_, err := d.Exec(`
INSERT INTO task_report_settings(task_id, profile_id) VALUES ($1,$2)
ON CONFLICT (task_id) DO UPDATE SET profile_id=EXCLUDED.profile_id, updated_at=now()`, taskID, profileID)
	return err
}
