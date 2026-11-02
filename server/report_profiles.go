package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/Autumn-27/norma/llm"
)

// --- report profiles (formats) ---

// reportProfiles lists every report format.
func (s *Server) reportProfiles(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	list, err := pg.ListReportProfiles()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"profiles": list})
}

// reportProfileSave creates (id=0) or updates a report format.
func (s *Server) reportProfileSave(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	var req struct {
		ID           int64  `json:"id"`
		Name         string `json:"name"`
		Instructions string `json:"instructions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.ID > 0 {
		if existing, err := pg.GetReportProfile(req.ID); err == nil && existing != nil && existing.Builtin {
			// The built-in format may be edited (an operator may want a different
			// standard), but it is never renamed into a second built-in.
			if _, err := pg.SaveReportProfile(req.ID, existing.Name, req.Instructions); err != nil {
				writeErr(w, http.StatusInternalServerError, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"id": req.ID})
			return
		}
	}
	id, err := pg.SaveReportProfile(req.ID, name, req.Instructions)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id})
}

// reportProfileDelete removes a custom format.
func (s *Server) reportProfileDelete(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	id, _ := strconv.ParseInt(r.PathValue("pid"), 10, 64)
	if err := pg.DeleteReportProfile(id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// taskReportSettings returns the report format selected for a task.
func (s *Server) taskReportSettings(w http.ResponseWriter, r *http.Request) {
	t := s.m.ResolveTask(r.PathValue("id"))
	if t == nil {
		writeErr(w, http.StatusNotFound, "task not found")
		return
	}
	taskID, _ := strconv.ParseInt(t.ID, 10, 64)
	writeJSON(w, http.StatusOK, map[string]any{"profile_id": s.m.pg.TaskReportProfile(taskID)})
}

// taskReportSettingsSave selects a report format for a task.
func (s *Server) taskReportSettingsSave(w http.ResponseWriter, r *http.Request) {
	t := s.m.ResolveTask(r.PathValue("id"))
	if t == nil {
		writeErr(w, http.StatusNotFound, "task not found")
		return
	}
	var req struct {
		ProfileID int64 `json:"profile_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	taskID, _ := strconv.ParseInt(t.ID, 10, 64)
	if err := s.m.pg.SetTaskReportProfile(taskID, req.ProfileID); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// renderReportForProfile turns the canonical report into the shape the selected format
// asks for. The built-in format is the canonical text itself; a custom format is a
// rewrite of that text, so the facts always come from the graph.
func (s *Server) renderReportForProfile(ctx context.Context, t *Task, canonical string, profileID int64) (string, error) {
	if profileID == 0 {
		return canonical, nil
	}
	p, err := s.m.pg.GetReportProfile(profileID)
	if err != nil || p == nil || p.Builtin {
		return canonical, nil
	}
	prov, ok := s.reportProvider(t)
	if !ok {
		return canonical, nil // no model: keep the canonical report rather than fail
	}
	temp := 0.0
	req := llm.CompletionRequest{
		System: []string{reportReviseSystem + "\n\nReport format to follow:\n" + p.Instructions},
		Messages: []llm.Message{llm.UserText(
			"Rewrite the following report in the requested format. Keep every fact, finding and piece of evidence; only change the structure and wording.\n\n" + canonical)},
		MaxTokens:   16000,
		Temperature: &temp,
	}
	var sb strings.Builder
	for ev, err := range prov.Stream(ctx, req) {
		if err != nil {
			return canonical, err
		}
		if ev.Type == llm.SETextDelta {
			sb.WriteString(ev.Text)
		}
	}
	out := strings.TrimSpace(sb.String())
	if out == "" {
		return canonical, nil
	}
	return out, nil
}
