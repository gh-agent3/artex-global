package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/gh-agent3/artex-global/db"
)

// The manual CRUD endpoints of the constraint management in the overview plus the resolution of the
// injection-scope switches. Operating constraints (allow/deny) are written to the same task_constraints
// table as the agent-side set_constraints tool; here a human edits them in the UI. A constraint is only
// prompt context - adding/editing/deleting does NOT notify the planner; the next planning round reads the
// table on its own (a product decision). Every mutating handler goes through
// beginTaskOperation/decInflight so it cannot race a task deletion (same as the goal/intent CRUD).

// The settings keys of the injection-scope switches, all on by default (GetBool's second argument = true).
const (
	settingConstraintsInjectPlanner = "constraints_inject_planner"
	settingConstraintsInjectWorker  = "constraints_inject_worker"
)

// The user-facing error responses of the constraint CRUD handlers. writeErr shows them in the UI toast as
// they are. Terminology: constraint -> constraint (matching the ko.json promise/constraint wording),
// task -> task. The add/edit/delete wording is
// goals_api.go 의 errGoalTaskDeleting* 와 같은 문형이고, kind 검증은 intercept.go 의
// "값은 … 중 하나여야 합니다" 패턴을 따른다.
const (
	errConstraintTaskDeletingAdd    = "작업을 삭제하는 중이라 제약을 추가할 수 없습니다"
	errConstraintTaskDeletingEdit   = "작업을 삭제하는 중이라 제약을 수정할 수 없습니다"
	errConstraintTaskDeletingDelete = "작업을 삭제하는 중이라 제약을 삭제할 수 없습니다"
	errConstraintTextEmpty          = "제약 내용은 비워 둘 수 없습니다"
	errConstraintKindInvalid        = "kind 값은 allow 또는 deny 중 하나여야 합니다"
)

// constraintInjectPlanner / constraintInjectWorker report whether operating constraints are injected into
// the corresponding agent's system prompt (on by default). Passed as the resolver to the planner/worker
// and read every round, so the switch takes effect immediately.
func (s *Server) constraintInjectPlanner() bool {
	return s.m.pg.GetBool(settingConstraintsInjectPlanner, true)
}

func (s *Server) constraintInjectWorker() bool {
	return s.m.pg.GetBool(settingConstraintsInjectWorker, true)
}

// listConstraints returns all of the task's operating constraints (allow first, then deny).
func (s *Server) listConstraints(w http.ResponseWriter, r *http.Request) {
	t, ok := s.m.Task(r.PathValue("id"))
	if !ok {
		writeErr(w, 404, "task not found")
		return
	}
	rows, err := t.Store.ListConstraints()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"constraints": constraintDTOs(rows)})
}

// addConstraint adds one operating constraint manually (kind=allow|deny). The planner is not notified.
func (s *Server) addConstraint(w http.ResponseWriter, r *http.Request) {
	t, ok := s.m.Task(r.PathValue("id"))
	if !ok {
		writeErr(w, 404, "task not found")
		return
	}
	if !s.engine.beginTaskOperation(t.ID) {
		writeErr(w, 409, errConstraintTaskDeletingAdd)
		return
	}
	defer s.engine.decInflight(t.ID)

	var body struct {
		Text string `json:"text"`
		Kind string `json:"kind"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	text := strings.TrimSpace(body.Text)
	if text == "" {
		writeErr(w, 400, errConstraintTextEmpty)
		return
	}
	kind := normalizeConstraintKind(body.Kind)
	if kind == "" {
		writeErr(w, 400, errConstraintKindInvalid)
		return
	}
	id, err := t.Store.AddConstraint(kind, text, "human")
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, ConstraintDTO{ID: strconv.FormatInt(id, 10), Kind: kind, Text: text, Origin: "human"})
}

// editConstraint edits one constraint (kind + text) manually. The planner is not notified.
func (s *Server) editConstraint(w http.ResponseWriter, r *http.Request) {
	t, ok := s.m.Task(r.PathValue("id"))
	if !ok {
		writeErr(w, 404, "task not found")
		return
	}
	if !s.engine.beginTaskOperation(t.ID) {
		writeErr(w, 409, errConstraintTaskDeletingEdit)
		return
	}
	defer s.engine.decInflight(t.ID)

	cid, err := strconv.ParseInt(r.PathValue("cid"), 10, 64)
	if err != nil || cid <= 0 {
		writeErr(w, 400, "bad constraint id")
		return
	}
	var body struct {
		Text string `json:"text"`
		Kind string `json:"kind"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	text := strings.TrimSpace(body.Text)
	if text == "" {
		writeErr(w, 400, errConstraintTextEmpty)
		return
	}
	kind := normalizeConstraintKind(body.Kind)
	if kind == "" {
		writeErr(w, 400, errConstraintKindInvalid)
		return
	}
	if err := t.Store.UpdateConstraint(cid, kind, text); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, ConstraintDTO{ID: strconv.FormatInt(cid, 10), Kind: kind, Text: text})
}

// deleteConstraint deletes one constraint manually. The planner is not notified.
func (s *Server) deleteConstraint(w http.ResponseWriter, r *http.Request) {
	t, ok := s.m.Task(r.PathValue("id"))
	if !ok {
		writeErr(w, 404, "task not found")
		return
	}
	if !s.engine.beginTaskOperation(t.ID) {
		writeErr(w, 409, errConstraintTaskDeletingDelete)
		return
	}
	defer s.engine.decInflight(t.ID)

	cid, err := strconv.ParseInt(r.PathValue("cid"), 10, 64)
	if err != nil || cid <= 0 {
		writeErr(w, 400, "bad constraint id")
		return
	}
	if err := t.Store.DeleteConstraint(cid); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// normalizeConstraintKind lowercases + validates the kind; "" on invalid.
func normalizeConstraintKind(k string) string {
	k = strings.TrimSpace(strings.ToLower(k))
	if k == "allow" || k == "deny" {
		return k
	}
	return ""
}

// constraintDTOs converts db rows to the frontend shape.
func constraintDTOs(in []db.Constraint) []ConstraintDTO {
	out := make([]ConstraintDTO, 0, len(in))
	for _, c := range in {
		out = append(out, ConstraintDTO{
			ID:     strconv.FormatInt(c.ID, 10),
			Kind:   c.Kind,
			Text:   c.Text,
			Origin: c.Origin,
			TS:     rfc3339(c.CreatedAt),
		})
	}
	return out
}
