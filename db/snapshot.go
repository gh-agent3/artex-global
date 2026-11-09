package db

// Frozen evidence snapshots for the retest/verify conversation agents.
// Assembled in Go (not SQL jsonb functions) so both dialects produce the same
// shape and SQLite needs no jsonb emulation.

import (
	"database/sql"
	"encoding/json"
	"fmt"
)

// snapshotAsset is the per-asset slice of the snapshot. Deliberately small —
// the agents read it as context, and the full asset graph is one tool call away.
type snapshotAsset struct {
	ID    int64  `json:"id"`
	Type  string `json:"type"`
	Value string `json:"value"`
}

// snapshotConstraint mirrors Constraint for the snapshot payload.
type snapshotConstraint struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
}

// findingSnapshot builds the frozen snapshot for one finding: the finding row
// itself plus the assets it touches and the task constraints it ran under.
func (d *DB) findingSnapshot(findingID int64) (title string, snap json.RawMessage, err error) {
	f, err := d.GetFinding(findingID)
	if err != nil {
		return "", nil, err
	}
	if f == nil {
		return "", nil, sql.ErrNoRows
	}
	title = f.Name
	if title == "" {
		title = f.VulnClass
	}
	if title == "" {
		title = "uncategorized"
	}
	out := map[string]any{"finding": f, "assets": []snapshotAsset{}, "constraints": []snapshotConstraint{}}
	if len(f.AssetIDs) > 0 {
		marks := make([]string, len(f.AssetIDs))
		args := make([]any, len(f.AssetIDs))
		for i, id := range f.AssetIDs {
			marks[i] = fmt.Sprintf("$%d", i+1)
			args[i] = id
		}
		// assets has no single "value" column: the human identifier lives in
		// domain / ip / url / app_name / service_name depending on the type.
		rows, err := d.Query(`SELECT id, type, COALESCE(NULLIF(domain,''), NULLIF(ip,''), NULLIF(url,''), NULLIF(app_name,''), NULLIF(service_name,''), '')
FROM assets WHERE id IN (`+fmt.Sprintf("%s", joinComma(marks))+`)`, args...)
		if err != nil {
			return "", nil, err
		}
		var assets []snapshotAsset
		for rows.Next() {
			var a snapshotAsset
			if err := rows.Scan(&a.ID, &a.Type, &a.Value); err != nil {
				rows.Close()
				return "", nil, err
			}
			assets = append(assets, a)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return "", nil, err
		}
		if assets != nil {
			out["assets"] = assets
		}
	}
	if f.TaskID != nil {
		crows, err := d.Query(`SELECT c.kind, c.text FROM task_constraints c JOIN tasks t ON t.exploration_id = c.exploration_id WHERE t.id = $1`, *f.TaskID)
		if err != nil {
			return "", nil, err
		}
		var cons []snapshotConstraint
		for crows.Next() {
			var c snapshotConstraint
			if err := crows.Scan(&c.Kind, &c.Text); err != nil {
				crows.Close()
				return "", nil, err
			}
			cons = append(cons, c)
		}
		crows.Close()
		if err := crows.Err(); err != nil {
			return "", nil, err
		}
		if cons != nil {
			out["constraints"] = cons
		}
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "", nil, err
	}
	return title, b, nil
}

func joinComma(s []string) string {
	out := ""
	for i, v := range s {
		if i > 0 {
			out += ","
		}
		out += v
	}
	return out
}
