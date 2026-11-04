package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/gh-agent3/artex-global/db"
)

// goals_api.go 가 사용자에게 돌려주는 오류 응답 문구. 명령·요청 필드명(text·vulnclass·
// gid)·enum 은 원문 그대로 두고, 사람이 읽는 메시지만 한국어로 둔다. 영어 writeErr
// (task not found·invalid JSON·bad goal id)는 F3b 중국어 전용 범위라 보존한다.
const (
	errGoalTaskDeletingAdd    = "작업을 삭제하는 중이라 목표를 추가할 수 없습니다"
	errGoalTaskDeletingEdit   = "작업을 삭제하는 중이라 목표를 수정할 수 없습니다"
	errGoalTaskDeletingDelete = "작업을 삭제하는 중이라 목표를 삭제할 수 없습니다"
	errGoalTextEmpty          = "목표 내용은 비워 둘 수 없습니다"
	errGoalNotFound           = "목표를 찾을 수 없습니다"
	errGoalReadAfterAdd       = "목표를 저장한 뒤 읽지 못했습니다"
	errGoalReadAfterEdit      = "목표를 수정한 뒤 읽지 못했습니다"
)

// The manual CRUD endpoints of the goal management in the overview. They write the same goal nodes as
// the agent-side set_goals tool, but the entry point is a human editing in the UI. After an add or edit
// the revive-task logic is reused (admitTask resume: terminal -> running, unpause, queue if needed); a
// delete does NOT revive (a product decision). Every mutating handler goes through
// beginTaskOperation/decInflight so it cannot race a task deletion (same as the intent CRUD).

// listGoals returns all of the task's goals (text/vulnclass/state split out) for the goal-management card.
func (s *Server) listGoals(w http.ResponseWriter, r *http.Request) {
	t, ok := s.m.Task(r.PathValue("id"))
	if !ok {
		writeErr(w, 404, "task not found")
		return
	}
	goals, err := t.Store.ListByKind(db.KindGoal, 10000)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"goals": goalDTOs(goals)})
}

// addGoal adds a goal manually: store it (attached under the task root spawns) -> record a "a goal was
// added" trigger that wakes the planner -> revive the task so the planner re-judges it against the new goal.
func (s *Server) addGoal(w http.ResponseWriter, r *http.Request) {
	t, ok := s.m.Task(r.PathValue("id"))
	if !ok {
		writeErr(w, 404, "task not found")
		return
	}
	if !s.engine.beginTaskOperation(t.ID) {
		writeErr(w, 409, errGoalTaskDeletingAdd)
		return
	}
	defer s.engine.decInflight(t.ID)

	var body struct {
		Text      string `json:"text"`
		VulnClass string `json:"vulnclass"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	text := strings.TrimSpace(body.Text)
	if text == "" {
		writeErr(w, 400, errGoalTextEmpty)
		return
	}
	payload := map[string]any{"text": text}
	if vc := strings.TrimSpace(body.VulnClass); vc != "" {
		payload["vulnclass"] = vc
	}
	id, err := t.Store.AddGoal(payload, "human")
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if of, _ := t.Store.OriginFactID(); of > 0 && id > 0 {
		_ = t.Store.Link(of, db.RelSpawns, id) // goal descends from the task root (origin fact)
	}
	t.NotifyGoal([]string{text}) // record a "the human added a goal: ..." trigger and wake the planner
	node, _ := t.Store.GetNode(id)
	if node == nil {
		writeErr(w, 500, errGoalReadAfterAdd)
		return
	}
	writeJSON(w, 200, goalDTO(node))
}

// editGoal edits a goal text (and vulnclass) manually: update the store -> record a "the user changed the
// goal from old to new" trigger that wakes the planner -> revive the task so the planner adjusts the
// direction for the new goal.
func (s *Server) editGoal(w http.ResponseWriter, r *http.Request) {
	t, ok := s.m.Task(r.PathValue("id"))
	if !ok {
		writeErr(w, 404, "task not found")
		return
	}
	if !s.engine.beginTaskOperation(t.ID) {
		writeErr(w, 409, errGoalTaskDeletingEdit)
		return
	}
	defer s.engine.decInflight(t.ID)

	gid, err := strconv.ParseInt(r.PathValue("gid"), 10, 64)
	if err != nil || gid <= 0 {
		writeErr(w, 400, "bad goal id")
		return
	}
	var body struct {
		Text      string `json:"text"`
		VulnClass string `json:"vulnclass"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	text := strings.TrimSpace(body.Text)
	if text == "" {
		writeErr(w, 400, errGoalTextEmpty)
		return
	}
	node, err := t.Store.GetNode(gid)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if node == nil || node.Kind != db.KindGoal {
		writeErr(w, 404, errGoalNotFound)
		return
	}
	oldText := goalDTO(node).Text
	if err := t.Store.UpdateGoalPayload(gid, text, strings.TrimSpace(body.VulnClass)); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	t.NotifyGoalEdited(oldText, text) // record a "the human changed the goal from old to new" trigger and wake the planner
	updated, _ := t.Store.GetNode(gid)
	if updated == nil {
		writeErr(w, 500, errGoalReadAfterEdit)
		return
	}
	writeJSON(w, 200, goalDTO(updated))
}

// deleteGoal deletes a goal manually (a hard delete, cascading edges/anchors): delete from the store ->
// record a "the user deleted the goal X" trigger that wakes the planner so it re-judges the remaining
// goals. Per the product decision a delete does NOT revive the task.
func (s *Server) deleteGoal(w http.ResponseWriter, r *http.Request) {
	t, ok := s.m.Task(r.PathValue("id"))
	if !ok {
		writeErr(w, 404, "task not found")
		return
	}
	if !s.engine.beginTaskOperation(t.ID) {
		writeErr(w, 409, errGoalTaskDeletingDelete)
		return
	}
	defer s.engine.decInflight(t.ID)

	gid, err := strconv.ParseInt(r.PathValue("gid"), 10, 64)
	if err != nil || gid <= 0 {
		writeErr(w, 400, "bad goal id")
		return
	}
	node, err := t.Store.GetNode(gid)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if node == nil || node.Kind != db.KindGoal {
		writeErr(w, 404, errGoalNotFound)
		return
	}
	text := goalDTO(node).Text
	if err := t.Store.DeleteGoal(gid); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	t.NotifyGoalDeleted(text) // record a "the human deleted the goal: ..." trigger and wake the planner (no revive)
	writeJSON(w, 200, map[string]bool{"ok": true})
}
