package server

// Harness editor (phase 1 of auto harness refinement, ADR-0002): a seeded
// conversation agent turns adopted lessons into ONE concrete prompt-template
// proposal. Nothing is applied automatically — proposals land in
// harness_proposals and the operator applies/rejects them in the UI (Lessons
// page → Proposals). Apply bumps the agent prompt version via SavePrompt, so
// roll-back is the prompt versions page. Phase 2 (A/B probes on future tasks)
// is deliberately deferred — recorded in ADR-0002.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"artex-global/agent"
	"artex-global/db"
	actool "github.com/Autumn-27/norma/tool"
)

const editorTriggerInterval = 6 * time.Hour

// editorTriggerLoop periodically offers the editor a batch of unproposed
// lessons. Gated on zero pending proposals: the operator decides before the
// next round, proposals never pile up.
func (s *Server) editorTriggerLoop(ctx context.Context) {
	tick := time.NewTicker(editorTriggerInterval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			s.maybeRunEditor(ctx)
		}
	}
}

func (s *Server) maybeRunEditor(ctx context.Context) {
	if s.m == nil || s.m.pg == nil {
		return
	}
	pending, err := s.m.pg.PendingHarnessProposalsExist()
	if err != nil || pending {
		return
	}
	batch, err := s.m.pg.LessonsWithoutProposals("medium", 10)
	if err != nil || len(batch) == 0 {
		return
	}
	if s.resolveChatAgent(&db.Conversation{AgentKey: db.HarnessEditorAgentKey}) == nil {
		return // editor agent disabled or chat unavailable — retry next tick
	}
	conv, err := s.m.pg.CreateConversation(db.HarnessEditorAgentKey, "harness edit proposals", nil)
	if err != nil {
		log.Printf(logA("[editor] conversation: %v"), err)
		return
	}
	busyKey := s.convBusyKey(conv.ID)
	s.chatMu.Lock()
	s.chatBusy[busyKey] = true
	s.chatMu.Unlock()
	s.runConversation(conv, "Review the lesson batch and propose at most one prompt edit. Start with get_lesson_batch.", busyKey)
}

// editorTools are the editor's fixed surface: read the batch, write a proposal.
func (s *Server) editorTools() []actool.CoreTool {
	return []actool.CoreTool{
		roTool("get_lesson_batch", "Read the adopted harness lessons that have no proposal yet (cue/lesson/evidence/confidence/domain). Takes no parameters.",
			objSchema(map[string]any{}), func(ctx context.Context, _ json.RawMessage) (actool.Result, error) {
				batch, err := s.m.pg.LessonsWithoutProposals("medium", 10)
				if err != nil {
					return actool.Errorf(err.Error()), nil
				}
				return jsonResult(map[string]any{"lessons": batch})
			}),
		wrTool("propose_prompt_edit", "Save one prompt-edit proposal for the operator to approve. The template is the COMPLETE new prompt body; every {{.Variable}} placeholder must be preserved verbatim. Change one thing.",
			objSchema(map[string]any{
				"agent_key": strParam("target agent: planner | worker | mainagent"),
				"summary":   strParam("one imperative line naming the change"),
				"rationale": strParam("which lesson ids/cues justify it and why it is safe"),
				"template":  strParam("the full new prompt template with all placeholders intact"),
			}, "agent_key", "summary", "rationale", "template"), func(ctx context.Context, in json.RawMessage) (actool.Result, error) {
				var a struct {
					AgentKey, Summary, Rationale, Template string
				}
				if err := json.Unmarshal(in, &a); err != nil {
					return actool.Errorf(err.Error()), nil
				}
				if a.AgentKey != "planner" && a.AgentKey != "worker" && a.AgentKey != "mainagent" {
					return actool.Errorf("agent_key must be planner, worker or mainagent"), nil
				}
				// Template validation — the same bar as a manual UI edit.
				ag, err := s.m.pg.GetAgentByKey(a.AgentKey)
				if err != nil || ag == nil {
					return actool.Errorf("target agent " + a.AgentKey + " not found"), nil
				}
				vars, _ := s.m.pg.PromptVars(ag.ID)
				if bad := validateTemplate(a.Template, withGlobalVars(vars)); bad != "" {
					return actool.Errorf("template invalid: " + bad), nil
				}
				id, created, err := s.m.pg.CreateHarnessProposal(ctx, a.AgentKey, a.Summary, a.Rationale, a.Template, lessonIDsFromRationale(a.Rationale))
				if err != nil {
					return actool.Errorf(err.Error()), nil
				}
				if !created {
					return actool.Errorf("an identical proposal for " + a.AgentKey + " is already pending; refine or wait for the operator decision"), nil
				}
				return jsonResult(map[string]any{"saved": true, "proposal_id": id})
			}),
	}
}

var lessonIDRe = regexp.MustCompile(`\b\d{1,9}\b`)

// lessonIDsFromRationale best-effort extracts lesson ids the editor cited, so
// the proposal keeps an honest link to its evidence. Numbers above a sane id
// range are ignored.
func lessonIDsFromRationale(rationale string) []int64 {
	seen := map[int64]bool{}
	var out []int64
	for _, m := range lessonIDRe.FindAllString(rationale, -1) {
		if n, err := strconv.ParseInt(m, 10, 64); err == nil && n > 0 && n < 1_000_000_000 && !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	if len(out) > 20 {
		out = out[:20]
	}
	return out
}

// seedHarnessEditor seeds the editable editor agent + its tools once.
func (s *Server) seedHarnessEditor() error {
	for _, t := range s.editorTools() {
		schema, _ := json.Marshal(t.InputSchema())
		bindings, _ := json.Marshal([]string{db.HarnessEditorAgentKey})
		if err := s.m.pg.SeedTool(t.Name(), t.Description(), schema, bindings); err != nil {
			return err
		}
	}
	const flag = "harness_editor_seed_v1"
	tx, err := s.m.pg.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`SELECT pg_advisory_xact_lock(7337741013)`); err != nil {
		return err
	}
	var done string
	err = tx.QueryRow(`SELECT value FROM settings WHERE key=$1`, flag).Scan(&done)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if done == "true" {
		return nil
	}
	var id int64
	err = tx.QueryRow(`INSERT INTO agents(key,name,description,role,builtin,enabled)
	VALUES ($1,'harness editor','Runs periodically; turns adopted lessons into at most one concrete prompt-edit proposal for operator approval. Nothing is applied automatically.','assistant',false,true)
	ON CONFLICT (key) DO NOTHING RETURNING id`, db.HarnessEditorAgentKey).Scan(&id)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if id > 0 {
		var pid int64
		if err = tx.QueryRow(`INSERT INTO agent_prompts(agent_id,version,template_text,note,updated_by)
		VALUES ($1,1,$2,'built-in default','system') RETURNING id`, id, agent.HarnessEditorDefaultPrompt).Scan(&pid); err != nil {
			return err
		}
		if _, err = tx.Exec(`UPDATE agents SET current_prompt_id=$1 WHERE id=$2`, pid, id); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(`INSERT INTO settings(key,value) VALUES ($1,'true') ON CONFLICT(key) DO UPDATE SET value='true'`, flag); err != nil {
		return err
	}
	return tx.Commit()
}

// ── API ──────────────────────────────────────────────────────────────────────

func (s *Server) listHarnessProposals(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	status := r.URL.Query().Get("status")
	rows, err := pg.ListHarnessProposals(status, 200)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"proposals": rows})
}

func (s *Server) applyHarnessProposal(w http.ResponseWriter, r *http.Request) {
	s.decideHarnessProposal(w, r, true)
}

func (s *Server) rejectHarnessProposal(w http.ResponseWriter, r *http.Request) {
	s.decideHarnessProposal(w, r, false)
}

func (s *Server) decideHarnessProposal(w http.ResponseWriter, r *http.Request, apply bool) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	id, ok := pathInt(r, "id")
	if !ok || id <= 0 {
		writeErr(w, 400, "bad proposal id")
		return
	}
	p, err := pg.GetHarnessProposal(id)
	if err != nil || p == nil {
		writeErr(w, 404, "proposal not found")
		return
	}
	if p.Status != "pending" {
		writeErr(w, 409, "proposal already decided")
		return
	}
	if !apply {
		changed, err := pg.DecideHarnessProposal(id, "rejected", 0)
		if err != nil || !changed {
			writeErr(w, 500, "could not reject")
			return
		}
		writeJSON(w, 200, map[string]any{"status": "rejected"})
		return
	}
	ag, err := pg.GetAgentByKey(p.AgentKey)
	if err != nil || ag == nil {
		writeErr(w, 404, "target agent not found")
		return
	}
	ver, err := pg.SavePrompt(ag.ID, p.Template, "harness proposal #"+strconv.FormatInt(id, 10)+": "+p.Summary, "harness-editor")
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	changed, err := pg.DecideHarnessProposal(id, "applied", int64(ver))
	if err != nil || !changed {
		writeErr(w, 500, "prompt saved but proposal state lost — check prompt versions")
		return
	}
	writeJSON(w, 200, map[string]any{"status": "applied", "prompt_version": ver})
}
