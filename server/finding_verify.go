package server

// Independent finding verification (P2 verifier). A scanner applies a severity
// policy to freshly recorded findings (critical/high always, medium by a
// deterministic sample, low never), enqueues a verification with a frozen
// evidence snapshot, and a runner drives a dedicated read-only "verifier"
// conversation agent per item. The verdict is mirrored onto findings.validation.
// Complements the report_finding verification gate (pre-filing) with a
// post-filing independent check — the model that filed the finding is not the
// model that verifies it.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"artex-global/agent"
	"artex-global/db"
	"artex-global/internal/intercept"
	actool "github.com/Autumn-27/norma/tool"
)

// verifyPolicy knobs. Medium findings are sampled deterministically (hash of
// the finding id) so the decision is stable across restarts and never needs a
// RNG at the policy site.
const (
	verifyMediumSamplePercent = 50
	verifyScanInterval        = 30 * time.Second
	verifyBatchLimit          = 25
)

func verifyPolicyApplies(severity string, findingID int64) bool {
	switch severity {
	case "critical", "high":
		return true
	case "medium":
		return int((findingID*2654435761)%100) < verifyMediumSamplePercent
	default:
		return false
	}
}

// verifyScanner periodically enqueues verifications for findings that have none.
func (s *Server) verifyScanner(ctx context.Context) {
	tick := time.NewTicker(verifyScanInterval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			s.verifyScanOnce(ctx)
		}
	}
}

func (s *Server) verifyScanOnce(ctx context.Context) {
	rows, err := s.m.pg.FindingsPendingVerification(ctx, verifyBatchLimit)
	if err != nil {
		log.Printf(logA("[verify] scan: %v"), err)
		return
	}
	for _, r := range rows {
		if !verifyPolicyApplies(r.Severity, r.ID) {
			continue
		}
		v, conv, created, err := s.m.pg.CreateFindingVerification(ctx, r.ID, "auto: severity policy")
		if err != nil {
			log.Printf(logA("[verify] enqueue finding %d: %v"), r.ID, err)
			continue
		}
		if !created {
			continue
		}
		busyKey := s.convBusyKey(conv.ID)
		s.chatMu.Lock()
		s.chatBusy[busyKey] = true
		s.chatMu.Unlock()
		s.runConversation(conv, db.VerificationInitialMessage(v.FindingID), busyKey)
	}
}

// verifyReaper seals rows left 'running' by a service restart so history stays honest.
func (s *Server) verifyReaper(ctx context.Context) {
	tick := time.NewTicker(5 * time.Minute)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			if _, err := s.m.pg.ExecContext(ctx, `
UPDATE finding_verifications SET status='failed', reason='service restarted mid-verification', finished_at=now()
WHERE status='running'`); err != nil {
				log.Printf(logA("[verify] reaper: %v"), err)
			}
		}
	}
}

// findingVerifyTools are the verifier's fixed read-only surface. Both tools
// derive the verification from server-owned conversation context — arguments
// cannot redirect a verdict into another finding or conversation.
func (s *Server) findingVerifyTools() []actool.CoreTool {
	return []actool.CoreTool{
		roTool("get_finding_verify_context", "Read the frozen finding snapshot (claim, severity, evidence, assets, original task constraints) bound to this verification conversation. Takes no parameters and can only read this conversation.",
			objSchema(map[string]any{}), func(ctx context.Context, _ json.RawMessage) (actool.Result, error) {
				convID := intercept.ConvIDFromContext(ctx)
				v, err := s.m.pg.VerificationForConversation(ctx, convID)
				if err != nil {
					return actool.Errorf(err.Error()), nil
				}
				if v == nil {
					return actool.Errorf("this conversation has no verification record bound"), nil
				}
				snap, err := s.m.pg.VerificationSnapshot(v.ID)
				if err != nil {
					return actool.Errorf(err.Error()), nil
				}
				var out map[string]any
				_ = json.Unmarshal(snap, &out)
				return jsonResult(map[string]any{"verification": v, "snapshot": out})
			}),
		wrTool("record_finding_verification_result", "Save the single verdict of this verification conversation. Call exactly once when done. The verdict is mirrored onto the finding's validation state; it never edits the finding itself.",
			objSchema(map[string]any{
				"verdict": map[string]any{"type": "string", "enum": []string{"verified", "refuted", "inconclusive"}},
				"reason":  strParam("one or two sentences naming the decisive evidence, the contradiction, or what is missing"),
			}, "verdict", "reason"), func(ctx context.Context, in json.RawMessage) (actool.Result, error) {
				var a struct {
					Verdict string `json:"verdict"`
					Reason  string `json:"reason"`
				}
				if err := json.Unmarshal(in, &a); err != nil {
					return actool.Errorf(err.Error()), nil
				}
				convID := intercept.ConvIDFromContext(ctx)
				v, err := s.m.pg.VerificationForConversation(ctx, convID)
				if err != nil || v == nil {
					return actool.Errorf("no verification record bound to this conversation"), nil
				}
				if v.Status != "pending" && v.Status != "running" {
					return actool.Errorf(fmt.Sprintf("verification %d already finished with status %q", v.ID, v.Status)), nil
				}
				if err := s.m.pg.FinishFindingVerification(v.ID, a.Verdict, a.Reason); err != nil {
					return actool.Errorf(err.Error()), nil
				}
				return jsonResult(map[string]any{"saved": true, "verdict": a.Verdict})
			}),
	}
}

// seedFindingVerifier seeds the editable verifier agent + its two tools exactly
// once; later edits survive restarts.
func (s *Server) seedFindingVerifier() error {
	for _, t := range s.findingVerifyTools() {
		schema, _ := json.Marshal(t.InputSchema())
		bindings, _ := json.Marshal([]string{db.FindingVerifierAgentKey})
		if err := s.m.pg.SeedTool(t.Name(), t.Description(), schema, bindings); err != nil {
			return err
		}
	}
	const flag = "finding_verifier_seed_v1"
	tx, err := s.m.pg.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`SELECT pg_advisory_xact_lock(7337741011)`); err != nil {
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
	VALUES ($1,'finding verifier','Runs automatically on recorded findings by severity policy; re-checks the frozen evidence read-only and saves an independent verdict.','assistant',false,true)
	ON CONFLICT (key) DO NOTHING RETURNING id`, db.FindingVerifierAgentKey).Scan(&id)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if id > 0 {
		var pid int64
		if err = tx.QueryRow(`INSERT INTO agent_prompts(agent_id,version,template_text,note,updated_by)
		VALUES ($1,1,$2,'built-in default','system') RETURNING id`, id, agent.VerifierDefaultPrompt).Scan(&pid); err != nil {
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

// listFindingVerifications exposes the verification history of one finding.
func (s *Server) listFindingVerifications(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	id, ok := pathInt(r, "id")
	if !ok || id <= 0 {
		writeErr(w, 400, "bad finding id")
		return
	}
	rows, err := pg.ListFindingVerifications(id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"verifications": rows})
}
