package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Autumn-27/artex/agent"
	"github.com/Autumn-27/artex/db"
	"github.com/Autumn-27/artex/internal/report"
	"github.com/Autumn-27/norma/llm"
)

// buildTaskReport renders the current report from the graph. It is the same input the
// plain /api/report endpoint uses, so version 1.0 is exactly what the tab showed before
// revisions existed.
func (s *Server) buildTaskReport(t *Task) string {
	findings, _ := t.Store.ListByKind(db.KindFinding, 1000)
	counts := map[string]int{}
	for _, ty := range []string{"root_domain", "ip", "subdomain", "app", "service", "endpoint"} {
		ns, _ := s.m.Assets().QueryByType(ty, 100000, 0)
		if len(ns) > 0 {
			counts[ty] = len(ns)
		}
	}
	return report.Markdown(report.Input{
		Title: t.Description, Goal: t.Goal, GeneratedAt: time.Now(),
		AssetCounts: counts, Findings: findings,
	})
}

// ensureReportVersion returns the newest stored version, seeding 1.0 from the graph on
// first use so the version tree always has a root.
func (s *Server) ensureReportVersion(t *Task) (*db.TaskReportVersion, error) {
	taskID, err := strconv.ParseInt(t.ID, 10, 64)
	if err != nil {
		return nil, err
	}
	if v, err := s.m.pg.LatestReportVersion(taskID); err == nil && v != nil {
		return v, nil
	}
	md := s.buildTaskReport(t)
	profileID := s.m.pg.TaskReportProfile(taskID)
	rendered, rerr := s.renderReportForProfile(context.Background(), t, md, profileID)
	if rerr != nil || strings.TrimSpace(rendered) == "" {
		rendered = md
	}
	id, err := s.m.pg.SaveReportVersion(taskID, "1.0", rendered, "", nil, profileID)
	if err != nil {
		return nil, err
	}
	return s.m.pg.GetReportVersion(taskID, id)
}

// taskReports lists a task's report versions (seeding 1.0 on first call).
func (s *Server) taskReports(w http.ResponseWriter, r *http.Request) {
	t := s.m.ResolveTask(r.PathValue("id"))
	if t == nil {
		writeErr(w, http.StatusNotFound, "task not found")
		return
	}
	if _, err := s.ensureReportVersion(t); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	taskID, _ := strconv.ParseInt(t.ID, 10, 64)
	list, err := s.m.pg.ListReportVersions(taskID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"versions": list})
}

// taskReportVersion returns one version with its Markdown.
func (s *Server) taskReportVersion(w http.ResponseWriter, r *http.Request) {
	t := s.m.ResolveTask(r.PathValue("id"))
	if t == nil {
		writeErr(w, http.StatusNotFound, "task not found")
		return
	}
	taskID, _ := strconv.ParseInt(t.ID, 10, 64)
	vid, _ := strconv.ParseInt(r.PathValue("rid"), 10, 64)
	v, err := s.m.pg.GetReportVersion(taskID, vid)
	if err != nil || v == nil {
		writeErr(w, http.StatusNotFound, "report version not found")
		return
	}
	writeJSON(w, http.StatusOK, v)
}

// taskReportDownload serves one version as a Markdown attachment.
func (s *Server) taskReportDownload(w http.ResponseWriter, r *http.Request) {
	t := s.m.ResolveTask(r.PathValue("id"))
	if t == nil {
		writeErr(w, http.StatusNotFound, "task not found")
		return
	}
	taskID, _ := strconv.ParseInt(t.ID, 10, 64)
	vid, _ := strconv.ParseInt(r.PathValue("rid"), 10, 64)
	v, err := s.m.pg.GetReportVersion(taskID, vid)
	if err != nil || v == nil {
		writeErr(w, http.StatusNotFound, "report version not found")
		return
	}
	name := fmt.Sprintf("report-%s-%s.md", strings.ReplaceAll(t.ID, " ", "_"), v.Version)
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+name+"\"")
	_, _ = w.Write([]byte(v.Content))
}

// reportProvider resolves the provider used to revise a report: the task's own profile
// first, then the globally active one.
func (s *Server) reportProvider(t *Task) (llm.Provider, bool) {
	if t.LLMProfileID != nil {
		if p, _, ok := s.providerForProfile(*t.LLMProfileID); ok {
			return p, true
		}
	}
	if a, _ := s.m.pg.ActiveProfile(); a != nil {
		if p, _, ok := s.providerForProfile(a.ID); ok {
			return p, true
		}
	}
	return nil, false
}

// bumpMinor turns "1.0" into "1.1", "1.9" into "1.10", and anything unparseable into a
// timestamp-suffixed label so a version is never lost or duplicated.
func bumpMinor(current string) string {
	parts := strings.SplitN(current, ".", 2)
	if len(parts) == 2 {
		major, err1 := strconv.Atoi(parts[0])
		minor, err2 := strconv.Atoi(parts[1])
		if err1 == nil && err2 == nil {
			return fmt.Sprintf("%d.%d", major, minor+1)
		}
	}
	return current + ".1"
}

const reportReviseSystem = `You are the report writer agent of an authorised penetration-testing system. You receive a finished Markdown penetration-test report and one instruction from the operator describing what to change.

Rewrite the WHOLE report so the instruction is applied, keeping the existing structure and every finding that the instruction does not ask to change. Never invent findings, evidence, CVEs or command output that the report does not already contain; if the instruction asks for something the report has no evidence for, say so in the relevant section instead of fabricating it. Keep code blocks, commands, URLs and identifiers verbatim.

Return ONLY the revised Markdown document: no preamble, no explanation, no code fence around the whole document.`

// taskReportRevise applies an operator instruction to one report version and stores the
// result as a child version.
func (s *Server) taskReportRevise(w http.ResponseWriter, r *http.Request) {
	t := s.m.ResolveTask(r.PathValue("id"))
	if t == nil {
		writeErr(w, http.StatusNotFound, "task not found")
		return
	}
	var req struct {
		Instruction string `json:"instruction"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	instruction := strings.TrimSpace(req.Instruction)
	if instruction == "" {
		writeErr(w, http.StatusBadRequest, "instruction is required")
		return
	}
	taskID, _ := strconv.ParseInt(t.ID, 10, 64)
	vid, _ := strconv.ParseInt(r.PathValue("rid"), 10, 64)
	base, err := s.m.pg.GetReportVersion(taskID, vid)
	if err != nil || base == nil {
		writeErr(w, http.StatusNotFound, "report version not found")
		return
	}
	prov, ok := s.reportProvider(t)
	if !ok {
		writeErr(w, http.StatusServiceUnavailable, "no LLM profile available to revise the report")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()
	temp := 0.0
	req2 := llm.CompletionRequest{
		System:      []string{reportReviseSystem},
		Messages:    []llm.Message{llm.UserText("Report:\n\n" + base.Content + "\n\n---\nOperator instruction: " + instruction)},
		MaxTokens:   16000,
		Temperature: &temp,
	}
	var sb strings.Builder
	for ev, err := range prov.Stream(ctx, req2) {
		if err != nil {
			writeErr(w, http.StatusBadGateway, "revision failed: "+err.Error())
			return
		}
		if ev.Type == llm.SETextDelta {
			sb.WriteString(ev.Text)
		}
	}
	revised := strings.TrimSpace(sb.String())
	if revised == "" {
		writeErr(w, http.StatusBadGateway, "the model returned an empty revision")
		return
	}
	// Strip an accidental whole-document fence.
	if strings.HasPrefix(revised, "```") {
		if i := strings.Index(revised, "\n"); i > 0 {
			body := revised[i+1:]
			if strings.HasSuffix(strings.TrimSpace(body), "```") {
				body = strings.TrimSuffix(strings.TrimSpace(body), "```")
				revised = strings.TrimSpace(body)
			}
		}
	}
	next := bumpMinor(base.Version)
	id, err := s.m.pg.SaveReportVersion(taskID, next, revised, instruction, &base.ID, s.m.pg.TaskReportProfile(taskID))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	v, err := s.m.pg.GetReportVersion(taskID, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, v)
}

var _ = agent.Config{}
