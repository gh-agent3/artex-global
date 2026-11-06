package server

// Harness lessons: harnessing-level recursion ported from Google Harness²'s
// experience-library idea (no harness2 code — just the discipline):
//   - after a task finishes, a reflector conversation extracts at most one
//     durable lesson {cue, lesson, evidence, confidence};
//   - new tasks inject matching lessons (global + same root_domain) into the
//     planner and worker system prompts.
// The reflector reuses the verifier's machinery shape: seeded conversation
// agent, bound read-only context, scanner + reaper goroutines.

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"artex-global/agent"
	"artex-global/db"
	"artex-global/internal/intercept"
	actool "github.com/Autumn-27/norma/tool"
)

const (
	reflectMinAgeMinutes  = 10 // let late-arriving activity/verdicts settle
	reflectActivityFloor  = 10 // skip trivial runs
	reflectBatchLimit     = 5
	reflectScanInterval   = 2 * time.Minute
	lessonsPromptMaxItems = 6
)

// ── prompt injection ─────────────────────────────────────────────────────────

// lessonsBlock builds the Harness²-style lessons tail for one task. Empty when
// nothing matches. Rendered per worker/planner run; keep it short — the block
// is guidance, not orders.
func (s *Server) lessonsBlock(taskID string) string {
	id, err := strconv.ParseInt(strings.TrimSpace(taskID), 10, 64)
	if err != nil || id <= 0 || s.m == nil || s.m.pg == nil {
		return ""
	}
	domains, err := s.m.pg.TaskRootDomains(id)
	if err != nil {
		log.Printf(logA("[lessons] domains task %s: %v"), taskID, err)
		return ""
	}
	rows, err := s.m.pg.MatchingHarnessLessons(domains, lessonsPromptMaxItems)
	if err != nil || len(rows) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\n**Lessons from earlier engagements** (patterns adopted from finished tasks; apply where the cue matches the situation — they are guidance, not orders):")
	for _, r := range rows {
		scope := "global"
		if r.Domain != "" {
			scope = r.Domain
		}
		b.WriteString("\n- Cue: ")
		b.WriteString(oneLine(r.Cue, 220))
		b.WriteString(" → Lesson: ")
		b.WriteString(oneLine(r.Lesson, 260))
		b.WriteString(" (")
		b.WriteString(scope)
		if r.TaskID != nil {
			b.WriteString(", task #" + strconv.FormatInt(*r.TaskID, 10))
		}
		b.WriteString(", " + r.Confidence + ")")
	}
	return b.String()
}

func oneLine(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > max {
		s = s[:max] + "…"
	}
	return s
}

// ── scanner / reaper ─────────────────────────────────────────────────────────

func (s *Server) reflectScanner(ctx context.Context) {
	tick := time.NewTicker(reflectScanInterval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			s.reflectScanOnce(ctx)
		}
	}
}

func (s *Server) reflectScanOnce(ctx context.Context) {
	ids, err := s.m.pg.TasksPendingReflection(ctx, reflectMinAgeMinutes, reflectActivityFloor, reflectBatchLimit)
	if err != nil {
		log.Printf(logA("[reflect] scan: %v"), err)
		return
	}
	for _, taskID := range ids {
		reflID, created, err := s.m.pg.CreateHarnessReflection(ctx, taskID)
		if err != nil {
			log.Printf(logA("[reflect] claim task %d: %v"), taskID, err)
			continue
		}
		if !created {
			continue
		}
		conv, created, err := s.m.pg.CreateReflectionConversation(ctx, reflID, taskID)
		if err != nil {
			log.Printf(logA("[reflect] conversation task %d: %v"), taskID, err)
			continue
		}
		if !created {
			continue
		}
		busyKey := s.convBusyKey(conv.ID)
		s.chatMu.Lock()
		s.chatBusy[busyKey] = true
		s.chatMu.Unlock()
		s.runConversation(conv, db.ReflectionInitialMessageFor(taskID), busyKey)
	}
}

// ReflectionInitialMessageFor is exported through db for the runner message.
func (s *Server) reflectReaper(ctx context.Context) {
	tick := time.NewTicker(5 * time.Minute)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			// Conversations that ended without record_harness_lesson never flip
			// the row; a crashed mid-reflection is indistinguishable and the
			// unique task_id means no retry either way — seal both as no_entry.
			if _, err := s.m.pg.ExecContext(ctx, `
UPDATE harness_reflections SET status='no_entry', finished_at=now()
WHERE status='running' AND started_at < now() - interval '2 hours'`); err != nil {
				log.Printf(logA("[reflect] reaper: %v"), err)
			}
		}
	}
}

// ── reflector tools ──────────────────────────────────────────────────────────

func (s *Server) reflectorTools() []actool.CoreTool {
	return []actool.CoreTool{
		roTool("get_reflection_context", "Read the frozen summary of the finished task bound to this reflection conversation: description/goal/status, activity and tool-error counts, LLM call/error counts, findings by verification verdict, refuted-finding reasons, and the number of lessons already adopted. Takes no parameters and can only read this conversation.",
			objSchema(map[string]any{}), func(ctx context.Context, _ json.RawMessage) (actool.Result, error) {
				convID := intercept.ConvIDFromContext(ctx)
				r, err := s.m.pg.ReflectionForConversation(ctx, convID)
				if err != nil {
					return actool.Errorf(err.Error()), nil
				}
				if r == nil {
					return actool.Errorf("this conversation has no reflection record bound"), nil
				}
				rc, err := s.m.pg.GetReflectionRunContext(ctx, r.TaskID)
				if err != nil {
					return actool.Errorf(err.Error()), nil
				}
				return jsonResult(map[string]any{"reflection": r, "run": rc})
			}),
		wrTool("record_harness_lesson", "Save the single durable lesson of this reflection. Call at most once, and only when the run record supports a concrete, reusable contrast. cue = the SHAPE of the situation (a pattern that can fire on a different target), lesson = the operation naming what it concerns, evidence = what in this run supports it.",
			objSchema(map[string]any{
				"cue":        strParam("the shape of situation the lesson applies to — a transferable pattern, not this task's facts"),
				"lesson":     strParam("the operation to perform, naming what it concerns (probe discipline / verification / measurement / prompt / constraints)"),
				"evidence":   strParam("the concrete observations from this run that support it"),
				"confidence": map[string]any{"type": "string", "enum": []string{"high", "medium", "low"}},
			}, "cue", "lesson", "evidence", "confidence"), func(ctx context.Context, in json.RawMessage) (actool.Result, error) {
				var a struct {
					Cue, Lesson, Evidence, Confidence string
				}
				if err := json.Unmarshal(in, &a); err != nil {
					return actool.Errorf(err.Error()), nil
				}
				a.Cue, a.Lesson, a.Evidence, a.Confidence = strings.TrimSpace(a.Cue), strings.TrimSpace(a.Lesson), strings.TrimSpace(a.Evidence), strings.TrimSpace(a.Confidence)
				if a.Cue == "" || a.Lesson == "" || a.Evidence == "" {
					return actool.Errorf("cue, lesson and evidence are all required; if the record supports nothing durable, end the conversation WITHOUT this tool"), nil
				}
				if a.Confidence == "" {
					a.Confidence = "low"
				}
				convID := intercept.ConvIDFromContext(ctx)
				r, err := s.m.pg.ReflectionForConversation(ctx, convID)
				if err != nil || r == nil {
					return actool.Errorf("no reflection record bound to this conversation"), nil
				}
				if r.Status != "pending" && r.Status != "running" {
					return actool.Errorf("reflection " + strconv.FormatInt(r.ID, 10) + " already finished with status " + r.Status), nil
				}
				domains, _ := s.m.pg.TaskRootDomains(r.TaskID)
				domain := ""
				if len(domains) > 0 {
					domain = domains[0]
				}
				id, err := s.m.pg.RecordHarnessLesson(r.TaskID, domain, a.Cue, a.Lesson, a.Evidence, a.Confidence)
				if err != nil {
					return actool.Errorf(err.Error()), nil
				}
				outcome := "completed"
				if err := s.m.pg.FinishHarnessReflection(r.ID, outcome); err != nil {
					return actool.Errorf(err.Error()), nil
				}
				return jsonResult(map[string]any{"saved": true, "lesson_id": id, "domain": domain})
			}),
	}
}

// seedHarnessReflector seeds the editable reflector agent + its tools once.
func (s *Server) seedHarnessReflector() error {
	for _, t := range s.reflectorTools() {
		schema, _ := json.Marshal(t.InputSchema())
		bindings, _ := json.Marshal([]string{db.HarnessReflectorAgentKey})
		if err := s.m.pg.SeedTool(t.Name(), t.Description(), schema, bindings); err != nil {
			return err
		}
	}
	const flag = "harness_reflector_seed_v1"
	tx, err := s.m.pg.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`SELECT pg_advisory_xact_lock(7337741012)`); err != nil {
		return err
	}
	var done string
	err = tx.QueryRow(`SELECT value FROM settings WHERE key=$1`, flag).Scan(&done)
	if err != nil && err.Error() != "sql: no rows in result set" {
		return err
	}
	if done == "true" {
		return nil
	}
	var id int64
	err = tx.QueryRow(`INSERT INTO agents(key,name,description,role,builtin,enabled)
	VALUES ($1,'harness reflector','Runs automatically when a task finishes; reviews the run read-only and records at most one durable, transferable lesson for future tasks.','assistant',false,true)
	ON CONFLICT (key) DO NOTHING RETURNING id`, db.HarnessReflectorAgentKey).Scan(&id)
	if err != nil && err.Error() != "sql: no rows in result set" {
		return err
	}
	if id > 0 {
		var pid int64
		if err = tx.QueryRow(`INSERT INTO agent_prompts(agent_id,version,template_text,note,updated_by)
		VALUES ($1,1,$2,'built-in default','system') RETURNING id`, id, agent.ReflectorDefaultPrompt).Scan(&pid); err != nil {
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

// listHarnessLessons exposes adopted lessons for the API.
func (s *Server) listHarnessLessons(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	rows, err := pg.ListHarnessLessons(limit)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"lessons": rows})
}
