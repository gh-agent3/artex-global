package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"artex-global/db"
	"artex-global/internal/intercept"
	"github.com/Autumn-27/norma/agentcore"
	"github.com/Autumn-27/norma/llm"
	"github.com/Autumn-27/norma/permission"
	actool "github.com/Autumn-27/norma/tool"
	"github.com/Autumn-27/norma/transcript"
)

// Planner is the event-driven LLM planner (docs §4.3): each time the asset or
// exploration graph changes (debounced), it reads the exploration route, queries
// assets, judges whether the task goal is met, and emits 0..N exploration intents
// into the frontier. It is the sole intent generator.
type Planner struct {
	findingRecorder   FindingRecorder
	prov              llm.Provider
	model             string
	tx                *transcript.Store                      // raw LLM conversation persistence (nil = off)
	window            int                                    // context window in tokens (for compaction)
	windowFn          func() int                             // optional dynamic task-chain minimum
	maxTurns          int                                    // max agent turns per run (0 = unlimited)
	killWork          func(intentID int64) error             // engine callback to terminate a running work (nil = off)
	steerWork         func(intentID int64, msg string) error // engine callback to steer a running work mid-run (nil = off)
	proxyAddr         string                                 // recording proxy for WebFetch (empty = direct)
	proxyCACert       string                                 // recording proxy's CA cert path (HTTPS verify)
	webSearch         WebSearchOpts                          // web_search tool backend selection (off by default)
	workDir           string                                 // shared work dir (surfaced in prompt as artifact-output target)
	injectConstraints func() bool                            // resolver: inject task operation constraints into system prompt? (nil = yes)
	nonStreamingFn    func() bool                            // resolver: use non-streaming (Complete) path? (nil = streaming)
	noaEnabledFn      func() bool                            // resolver: use experimental noa compaction? (nil = off)
	maxTokensFn       func() int                             // resolver: per-reply output cap (nil/0 = send no cap)
	compactor         *Compactor                             // cold-node compaction (§7); nil = disabled

	// todos keeps ONE plan-scratchpad per task (keyed by exploration id) so the
	// planner's multi-step plan survives across wake-ups — each Plan() is a fresh
	// session, but the shared store lets it record a serial exploit chain once and
	// dispatch it step-by-step over rounds instead of front-loading it in parallel.
	todoMu sync.Mutex
	todos  map[int64]*actool.TodoStore
}

func NewPlanner(prov llm.Provider, model, workDir string, tx *transcript.Store, window, maxTurns int) *Planner {
	return &Planner{prov: prov, model: model, workDir: workDir, tx: tx, window: window, maxTurns: maxTurns, todos: map[int64]*actool.TodoStore{}}
}

func (p *Planner) SetCompactionWindowResolver(fn func() int) { p.windowFn = fn }

// SetCompactor wires the cold-node compactor (cold-digest §7). Called each
// planner wake-up to advance the round counter, maintain cold stamps, and
// (off the hot path) fold cold nodes into digests. nil = feature disabled.
func (p *Planner) SetCompactor(c *Compactor) { p.compactor = c }

// SetNonStreaming wires a resolver deciding whether runs use the non-streaming
// model path (true = non-streaming). nil/unset = streaming (default).
func (p *Planner) SetNonStreaming(fn func() bool) { p.nonStreamingFn = fn }

func (p *Planner) nonStreaming() bool { return p.nonStreamingFn != nil && p.nonStreamingFn() }

// SetNoaEnabled wires a resolver deciding whether runs use the experimental noa
// context-compression mechanism. nil/unset = off (built-in compaction). Read per
// run so the settings toggle takes effect without rebuilding the agent.
func (p *Planner) SetNoaEnabled(fn func() bool) { p.noaEnabledFn = fn }

// SetMaxTokens wires a resolver for the per-reply output cap. nil/unset or 0 =
// send no cap and let the endpoint decide. Read per run, like nonStreaming.
func (p *Planner) SetMaxTokens(fn func() int) { p.maxTokensFn = fn }

func (p *Planner) maxTokens() int {
	if p.maxTokensFn == nil {
		return 0
	}
	return p.maxTokensFn()
}

func (p *Planner) compactionWindow() int {
	if p.windowFn != nil {
		return p.windowFn()
	}
	return p.window
}

// SetProxy points the planner's WebFetch at the recording proxy plus the CA cert
// it trusts to verify HTTPS through it (empty addr = direct).
func (p *Planner) SetProxy(addr, caCert string) { p.proxyAddr, p.proxyCACert = addr, caCert }

// SetWebSearch selects the web_search backend for the planner (off by default).
func (p *Planner) SetWebSearch(o WebSearchOpts) { p.webSearch = o }

// SetConstraintInject wires a resolver deciding whether this task's operation
// constraints get injected into the planner system prompt. Read per round so the
// settings toggle takes effect without rebuilding the agent. nil = inject (default).
func (p *Planner) SetConstraintInject(fn func() bool) { p.injectConstraints = fn }

// wantConstraints reports whether constraint injection is enabled (default yes).
func (p *Planner) wantConstraints() bool { return p.injectConstraints == nil || p.injectConstraints() }

// todoFor returns the task's persistent planning todo store, creating it on first
// use. Shared across all of this task's planner wake-ups.
func (p *Planner) todoFor(expID int64) *actool.TodoStore {
	p.todoMu.Lock()
	defer p.todoMu.Unlock()
	s := p.todos[expID]
	if s == nil {
		s = actool.NewTodoStore()
		p.todos[expID] = s
	}
	return s
}

// SetKillWork wires the engine's per-work terminate callback so the planner's
// kill_work tool can stop a single running worker.
func (p *Planner) SetKillWork(fn func(intentID int64) error) { p.killWork = fn }

// SetSteerWork wires the engine's per-work steering callback so the planner's
// steer_work tool can inject a mid-run course-correction into a running worker.
func (p *Planner) SetSteerWork(fn func(intentID int64, msg string) error) { p.steerWork = fn }

// renderPlannerTodos formats the persistent planning todo for injection into the
// wake-up prompt (empty when there are no todos yet — first wake-up).
func renderPlannerTodos(items []actool.Todo) string {
	if len(items) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\n[Your planning todos (kept across wake-ups, written by you last round)]:\n")
	for _, it := range items {
		mark := map[actool.TodoStatus]string{actool.TodoPending: "☐", actool.TodoInProgress: "▶", actool.TodoCompleted: "✔"}[it.Status]
		if mark == "" {
			mark = "☐"
		}
		b.WriteString(fmt.Sprintf("  %s %s\n", mark, it.Content))
	}
	b.WriteString("Act on it: dispatch an intent only for the next step whose [prerequisites are done / the facts it depends on already exist]; update the list with TodoWrite (mark steps already satisfied by a fact as completed). Do not dispatch a step that is already pending/in_progress in the list.")
	return b.String()
}

// TriggerEvent describes what concretely caused this planning round to fire, so
// the planner looks first at the actual change instead of re-scanning the whole
// overview. Kind:
//
//	"done"    — a worker finished intent IntentID (its output conclusion is fetched).
//	"finding" - a worker reported a finding on intent IntentID (Detail = summary).
//	"goal"    - the human (via the main agent's set_goals) added one OR MORE goals in a
//	            single call (Goals = the goal texts added this time, 1+; set_goals supports batches).
//	"goal_deleted" - the human deleted a goal from the overview's goal management (Detail = the deleted goal text).
//	"goal_edited"  - the human edited a goal from the overview's goal management (OldGoal -> NewGoal texts).
//	"cancelled" - the human deleted intent IntentID (Detail = deletion reason). The intent is
//	            stopped (not deleted) and the reason is attached to it as a fact.
type TriggerEvent struct {
	Kind     string
	IntentID int64
	Detail   string
	Summary  string   // Kind=="cancelled" only: the intent summary captured before deletion (after a real delete the node is gone and cannot be looked up)
	Goals    []string // Kind=="goal" only: the goal texts added by this set_goals call (one or more)
	OldGoal  string   // Kind=="goal_edited" only: the goal text before the edit
	NewGoal  string   // Kind=="goal_edited" only: the goal text after the edit
	Hints    []string // Kind=="hint" only: the hint texts added by this add_hint call (one or more)
}

// renderTriggers spells out the change(s) that fired this round: for a finished
// worker — which intent + its output conclusion; for a finding — which intent +
// what was found. Empty for time/heartbeat wakes. Reads the store (best-effort;
// a blank field never blocks the round).
func renderTriggers(ts *db.ExplorationStore, evs []TriggerEvent) string {
	if len(evs) == 0 || ts == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\n[The actual change that triggered this round (read this first, then decide whether to add directions)]:")
	for _, ev := range evs {
		switch ev.Kind {
		case "goal":
			if len(ev.Goals) == 1 {
				b.WriteString(fmt.Sprintf("\n- The human (main agent) added one goal: %s - a new goal to reach, add exploration directions for it (if none exists yet).", ev.Goals[0]))
			} else {
				b.WriteString(fmt.Sprintf("\n- The human (main agent) added %d goals: %s - all new goals to reach, add exploration directions for each one that has none yet.", len(ev.Goals), strings.Join(ev.Goals, "; ")))
			}
		case "hint":
			if len(ev.Hints) == 1 {
				b.WriteString(fmt.Sprintf("\n- The human (main agent) added one strategic hint: %s - it is attached to the exploration graph, adjust/add exploration directions accordingly (if none exists yet).", ev.Hints[0]))
			} else {
				b.WriteString(fmt.Sprintf("\n- The human (main agent) added %d strategic hints: %s - all attached to the exploration graph, adjust/add exploration directions for each.", len(ev.Hints), strings.Join(ev.Hints, "; ")))
			}
		case "goal_deleted":
			b.WriteString(fmt.Sprintf("\n- The human deleted that goal: %s - the goal is removed, re-judge the remaining goals/directions accordingly (no more intents for it).", ev.Detail))
		case "goal_edited":
			b.WriteString(fmt.Sprintf("\n- The human edited a goal, from \"%s\" to \"%s\" - adjust the exploration direction to the new goal (stop dispatching the old one if it no longer applies).", ev.OldGoal, ev.NewGoal))
		case "finding":
			b.WriteString(fmt.Sprintf("\n- The worker of intent #%d (%s) reported a finding: %s", ev.IntentID, intentSummary(ts, ev.IntentID), ev.Detail))
		case "cancelled":
			// Prefer the Summary captured at deletion time for the intent content (after a real delete the node is gone, so intentSummary cannot find it).
			sm := ev.Summary
			if sm == "" {
				sm = intentSummary(ts, ev.IntentID)
			}
			b.WriteString(fmt.Sprintf("\n- Intent #%d was deleted by the user; its content was: %s, deletion reason: %s. The intent is deleted (it will not run); re-plan accordingly.", ev.IntentID, sm, ev.Detail))
		default: // "done"
			b.WriteString(fmt.Sprintf("\n- The worker of intent #%d (%s) finished, output conclusion: %s", ev.IntentID, intentSummary(ts, ev.IntentID), workerOutput(ts, ev.IntentID)))
			if fids := factIDsYielded(ts, ev.IntentID); fids != "" {
				b.WriteString(fmt.Sprintf("; new fact ids produced by this intent: %s ", fids))
			}
		}
	}
	b.WriteString("\n(For full details use node_detail / get_worker_output / list_findings.)")
	return b.String()
}

// factIDsYielded lists the fact ids an intent produced this run as "#12、#15", so the
// planner can jump straight to the round's incremental facts. Empty (best-effort) when
// the intent yielded no facts or the lookup fails.
func factIDsYielded(ts *db.ExplorationStore, id int64) string {
	ids, err := ts.FactsYielded(id)
	if err != nil || len(ids) == 0 {
		return ""
	}
	parts := make([]string, len(ids))
	for i, fid := range ids {
		parts[i] = fmt.Sprintf("#%d", fid)
	}
	return strings.Join(parts, "、")
}

// intentSummary reads an intent node's one-line summary (best-effort, "?" on miss).
func intentSummary(ts *db.ExplorationStore, id int64) string {
	n, err := ts.GetNode(id)
	if err != nil || n == nil {
		return "?"
	}
	var p map[string]any
	if json.Unmarshal(n.Payload, &p) == nil {
		if s, ok := p["summary"].(string); ok && s != "" {
			return s
		}
	}
	return "?"
}

// workerOutput returns the finished worker's conclusion for an intent — the last
// 'result' (else 'text') activity's full detail, truncated. Same source get_worker_output uses.
func workerOutput(ts *db.ExplorationStore, id int64) string {
	acts, _, err := ts.ActivityList(&id, 0, 1000)
	if err != nil {
		return "(failed to get output)"
	}
	var pick *db.Activity
	for i := range acts {
		if acts[i].Kind == "result" {
			pick = &acts[i]
		} else if acts[i].Kind == "text" && pick == nil {
			pick = &acts[i]
		}
	}
	if pick == nil {
		return "(this work has no output recorded)"
	}
	out, _ := ts.ActivityDetail(pick.ID)
	if out == "" {
		out = pick.Summary
	}
	return truncOutput(out, 800)
}

// truncOutput caps a worker-output blob so the trigger context doesn't bloat the
// system prompt every round; full text is one get_worker_output call away.
func truncOutput(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + " ... (truncated, see get_worker_output for the full text)"
}

// renderGraphOverview folds the pre-computed graph_overview snapshot into the
// wake-up prompt so the planner starts each round with the full situation in
// hand — saving the round-trip it would otherwise spend calling the tool. It is
// the exact same JSON graph_overview would return; deeper detail is still one
// tool call away (node_detail / list_facts / …).
func renderGraphOverview(data map[string]any) string {
	b, err := json.Marshal(data)
	if err != nil {
		return "" // fall back to the model calling graph_overview itself
	}
	return "\n\n[Current situation (graph_overview prefetched; identical to what that tool returns; call node_detail/list_facts etc. when you need details)]:\n" + string(b)
}

// plannerDefaultTmpl is the built-in EDITABLE body (segment [A]) of the planner prompt,
// seeded into agent_prompts. Goal is a {{.Goal}} template var; the intermediate-output contract
// tail is code-owned (artifactSpec) and appended by plannerSystem after rendering.
const plannerDefaultTmpl = `You are the "planner" of the authorized penetration-testing system of a network security platform, woken frequently (on every graph change). Your job: read the situation -> judge the goals -> **add exploration intents only when there really is an uncovered new direction**. You plan, you do not execute: everything you produce this round is either [create/state an intent] or [judge a goal]; never do the work inside plan.

Task goal: {{.Goal}}

**How many intents to produce this round (think this through first)**:
- **Hard floor (highest priority)**: whenever [the goal is not met] and [there is currently no open or running intent] (frontier_open=0 and running_intents empty), you [must] produce at least one intent that moves toward the goal this round - with no running work to wait for and no queued direction, 0 intents means the task stalls; even if every known direction lives only in recent_done, open a new one or dispatch a follow-up based on the done/exhausted/blocked judgement below.
- Beyond that floor, **producing 0 intents is a normal outcome, but it needs a real reason** (not the default "dispatching less is safer"): (1) **already covered** - every direction you can think of is already handled by an intent that is still open/running (regenerating an existing intent in different words is a serious error); (2) **waiting on a dependency** - the next step depends on output from work that is still running and has not produced it yet (forcing a dispatch here leaves the downstream step without its prerequisite and spinning, so wait for the next wake-up with a graph update).
- Conversely: if there really is a [new, uncovered direction that does not depend on running work], or the goal is not met and untested surface remains in scope, dispatch - do not treat 0 intents as the lazy default.

**Decision flow on every wake-up**:

1. **The full situation is attached below this prompt** (it is exactly what graph_overview returns, no need to call it again): task (original title + goal/root node), asset counts, goals + state, open/running/recent_done intents, sites_without_endpoints (sites with no endpoints, hinting at directions still to explore), facts (number of exploration facts, a different class from findings), recent_facts ({id,summary,confidence?}).
   - **Scope**: exploration nodes (goals/intents/facts/findings) cover this task only; the **asset graph is globally shared** (one copy across tasks, so asset counts are global within the scope, not unique to this task) - ignore assets unrelated to this task.
   - **Lineage**: every intent carries parents (upstream: which facts/intents it was derived from) and yields (downstream: which facts/findings it produced), and every recent_facts entry carries from_intent; use this to understand "which fact came from which direction, and whether a new direction can be synthesized".
   - **Negative/doubtful observations** (in recent_facts, e.g. "port closed / not injectable") are a worker's observations, not verdicts: before accepting one, look at its evidence with node_detail(id) - only when the evidence is solid, confidence=observed and the means are exhausted may that direction be treated as temporarily closed; when evidence is missing, it only "looks like it / was probed once", or confidence=inferred, treat it as [not yet established], and if it is in scope and no other intent covers it, dispatch one verification intent by default to confirm or refute it (**at most one recheck per negative direction**; if it is still negative afterwards with sound evidence, respect the conclusion and do not dispatch again).
   - **Call these only when you need deeper detail**: list_facts (paginated, newest first, default 20, q filter, before cursor, with total/has_more), list_findings (all findings), node_detail(id) (full evidence/details; the lists and recent_facts give summaries only), list_assets (pull: q search, type/company_id/task_id filters, pagination, or direct id/ids), asset_neighbors. Assets are globally shared, do not pull the full set by default.

2. **Judge the goals (your core job)**: the goals field already holds the goals and their state; for an unmet goal that a finding/fact proves, call prove_goal(goal_id, evidence_id, reason) to mark it met. **When the goal you mark is the last unfinished one, the system judges the whole task complete automatically** - closing is driven only by per-goal prove_goal, there is no other "one-click complete".
   - ⚠️ **Check quantitative acceptance criteria (never sign off early)**: when a goal has a quantitative condition (coverage reaches X%, capture N flags, obtain a certain privilege), before prove_goal you [must] check the measured value in the graph_overview above (coverage.pct, findings_total and so on): if it is not met you are [forbidden] to prove_goal, dispatch an intent to close the gap instead; never mark it met early on the grounds of "broadly achieved / the core is done". Example: 100% coverage required while the measured coverage.pct=40% -> not met, keep dispatching gap-filling intents.

3. **(Optional, opening only, very light) probing to understand**: only when the graph has almost no facts yet (recent_facts essentially empty, the task just started) and the situation alone is not enough to state the initial intent concretely, use Bash and similar for a very small, read-only probe of the target (e.g. 1-2 curl calls to see the front page/fingerprint). **The only legitimate product is a more precise intent description** - never the discovery/verification/exploitation of a finding, and never the result of enumerating endpoints/directories/parameters (those are the worker's job, write them as an intent and dispatch). Three hard boundaries:
   - If the graph already has facts produced by workers (facts>0 / recent_facts not empty) -> you are [forbidden] to probe yourself; every judgement rests on existing facts, and this round's product can only be "dispatch a new intent" or "finish"; to dig into a lead -> dispatch an intent so a worker investigates, do not curl yourself.
   - Even at the opening, probe at most 3 times and stop, only to state the initial intent clearly; as soon as you notice you are "investigating deeply" instead of "quickly setting direction" (enumerating endpoints/directories one by one, trying ids one by one, decoding chains, probing the same endpoint repeatedly, any injection/authorization-bypass/vulnerability verification - all heavy worker work), stop at once and write an intent.
   - If it can be judged from existing facts/situation, there is no need to probe at all.

4. **Decide which new directions to add**: **"restraint" here means only [do not repeat an existing intent], not "dispatch as few as possible"** - while the goal is unmet, the default question is "what deeper, harder, still uncovered approach brings us closer to the goal", not "can we wrap up". An intent is an [open exploration direction] (not a fixed type/menu); judge directions yourself from known facts, assets and goals, and compare each against open + running + recent_done:
   - Already covered by open/running -> do not generate (it is being handled).
   - Seen in recent_done -> **first read that intent's state (every entry carries one) to see how it stopped, then decide**:
     · **done (finished normally)**: already covered -> do not re-dispatch as-is; whether it is a dead end comes from the fact conclusions it yields, not from the state; re-dispatch only when [materially new mechanism] appears (new fact/asset/parameter, a clearly different approach), and say in summary how it differs from last time; rewording or "trying again might work" does not count, retrying is forbidden.
     · **exhausted (budget ran out, cut off halfway, only partial output written back) / blocked (model or network failure, barely explored)**: both ended badly with incomplete information - first use get_worker_trace / get_worker_output to see how far it actually got and where it stalled, then choose from: cut off by the budget close to a breakthrough -> dispatch "continue from the previous progress"; failed purely on an external fault (often blocked) -> re-dispatch the same direction directly; stalled at the same place every time -> change the approach/direction. The basis is always the real progress in the trace, never the state itself.
   - A brand-new direction that no intent covers -> generate.
   - Every known direction is covered by an intent still open/running -> do not generate, just finish (there is running/queued work, let it advance); but if only recent_done covers it, nothing is open/running and the goal is unmet -> the hard floor at the top requires opening or dispatching a follow-up.
   - **Depth before coverage**: coverage is a floor/acceptance item, not the exploration goal itself; once a high-value entry point is found (one that may lead to RCE/privilege escalation/data exfiltration), prefer dispatching an intent to [drive that path all the way through] rather than spreading wide and testing every asset shallowly just to level coverage.
   - **Keep the routes diverse, do not converge too early**: while the goal is unmet, if all existing intents crowd onto one route/entry point and a [fundamentally different] uncovered direction exists (another entry surface/another asset class/another exploitation chain), prefer that divergent direction over adding a synonymous intent on the same line (judge the substance, not the wording); if that divergent direction is already covered by an existing intent, still do not generate. The ideal is 2-3 routes with different mechanisms running side by side (e.g. "attack via the upload chain" and "attack via authentication bypass"), and only once one of them produces evidence of [progress toward the goal] do you concentrate resources on it. **But diversity always yields to the [operating constraints] at the top**: an entry surface/port/host/operation excluded by a constraint is never dispatched, however different it is.

   **Serial exploitation chains: dispatch step by step, do not split them into parallel intents.** For a strongly dependent serial chain (1->2->3, each step depending on the actual output of the previous one): do not dispatch it all at once in parallel (a downstream step without its prerequisite only repeats/spins); record the whole chain as todos with TodoWrite (one per step), dispatch only the step whose "prerequisites are met" this round (usually the first), and once it produces a fact, on the next wake-up (the prompt carries the todo list) dispatch the next step and mark the satisfied ones completed. Do not split "the same thing" into two intents ("confirm the trigger point" and "trigger the trigger point" are the same step); only [parallel, mutually independent] dimensions (e.g. enumerating several unrelated endpoints) may run as parallel intents.

5. **Submit**: submit the selected new directions with [one] batched add_intent call (intents array, at most the 4 most valuable, do not call once per item):
   - **summary**: one natural-language sentence describing the direction (full address of the test target + what to do + why), without forcing a fixed taxonomy; dedup relies mainly on comparing it with existing intents.
   - **asset_ids**: the target asset ids this direction will test/attack (pass them whenever possible, 0/1/many, from list_assets) - whenever the direction revolves around concrete assets (sites/endpoints/parameters/hosts) you must pass them, for coverage dedup and for linking into the asset chain; pass them all when it spans several assets; leave empty only for pure global reconnaissance with no concrete asset.
   - **parent_ids**: which upstream nodes this direction was synthesized from (optional, 0/1/many) - pass all of them when several facts jointly produce one intent, also pass the id when it derives from an upstream intent/finding, leave empty for a brand-new top-level direction.

Do not repeat, do not pad; but when the goal is unmet and a deeper uncovered approach exists, dispatch it. Be concise, focused and efficient.`

func plannerSystem(goal, dataDir, workDir string) string {
	body := renderSystem("planner", plannerDefaultTmpl, PlannerVars{Goal: goal, DataDir: dataDir, Now: nowStr()})
	return body + artifactSpec(workDir) + measurementSpec() + langDirective(resolveOutputLang())
}

// Plan runs one planning round. emit, if non-nil, receives the planner's execution
// steps (so users can see how it reads the situation and judges goals — the
// planner is the intent generator and was previously a black box). Returns whether
// the planner judged the goal met.
// triggers carries the concrete change(s) that fired this round — worker(s) done
// and/or finding(s) reported (may be several — the engine debounces a burst; empty
// for time/heartbeat wakes). They are spelled out at the top of the prompt so the
// planner looks first at the actual change (which intent, its output/finding).
func (p *Planner) Plan(ctx context.Context, taskID int64, as *db.AssetStore, ts *db.ExplorationStore, goal string, triggers []TriggerEvent, emit func(db.Activity)) (met bool, reason string, err error) {
	// cold-digest §2.3/§7: advance this task's planner-round counter, maintain the
	// cold_since_round stamps, and (if a threshold is hit) kick off background
	// compaction. Synchronous part is cheap (a few queries); the LLM compaction
	// runs in a detached goroutine so it never adds latency to this round.
	p.compactor.OnPlannerRound(ctx, ts)
	tsx := NewToolSet(ts, "planner")
	tsx.SetFindingRecorder(p.findingRecorder)
	if as != nil {
		tsx.SetAssetStore(as, as.Companies())
	}
	tsx.SetTaskID(taskID)
	tsx.SetCoverageEnabled(as == nil || as.CoverageEnabled(taskID))
	tsx.killWork = p.killWork   // enable kill_work tool (nil = unavailable)
	tsx.steerWork = p.steerWork // enable steer_work tool (nil = unavailable)
	if origin, _ := ts.OriginFactID(); origin > 0 {
		tsx.SetOwnerNode(origin) // planner-side anchors default to the task root (origin fact)
	}
	// Domain tools + the base default tool set (Read/Write/Edit/MultiEdit/LS/Glob/Grep/Bash)
	// With the asset-coverage feature off, add_task_scope/list_untested_assets are removed (not put into the prompt).
	base := append(tsx.DropCoverageTools(tsx.PlannerTools()), actool.DefaultTools()...)
	ctx = WithRunInfo(ctx, RunInfo{TaskID: taskID, ExplorationID: explorationID(ts)})
	tools, def, cleanup := AugmentTools(ctx, "planner", base)
	defer cleanup()
	// Key situation (the intent just finished + the prefetched full graph) moved into [this round's user input] (see input below); system
	// keeps only the static planning body. Moving it out keeps system stable across rounds, which helps caching; the cost is that if a single
	// round gets long the situation may be compacted (planner rounds are usually short, so the risk is low). situational is appended to input below.
	situational := renderTriggers(ts, triggers) + renderGraphOverview(tsx.graphOverviewData())
	// Task-level deadline / endgame mode (injected via ctx, see taskclock.go). In the endgame round the task-timeout
	// planner wrap-up wording is appended to this round's user input as a [this-round instruction] (together with situational), so it only
	// does the final goal judgement and produces no new intents.
	tc := taskClockFrom(ctx)
	if tc.Final {
		situational += "\n\n[Task endgame wrap-up (special instruction for this round, overriding the normal planning flow above)]: " + resolveTaskTimeoutWrapup("planner")
	}
	// This task's working directory <workDir>/tasks/<taskID>, created first.
	taskDir := ensureRunDir(p.workDir, taskID, 0)
	ctx = intercept.WithReviewContext(ctx, taskDir, intercept.ReviewBackground{})
	sysBody := plannerSystem(goal, p.workDir, taskDir)
	// Harnessing-level recursion (see worker.go): lessons from earlier tasks.
	if LessonsProvider != nil {
		if b := LessonsProvider(strconv.FormatInt(taskID, 10)); b != "" {
			sysBody += b
		}
	}
	if p.wantConstraints() {
		sysBody += constraintBlock(ts) // operating constraints (if any) injected into the system prompt to bound the exploration
	}
	system, boundary := deferredSystem(sysBody, def)
	// The planner has no wall-clock budget of its own; with a deadline, MaxDuration is clamped to the remaining time so a running planning
	// round settles when the task times out (timeout -> task-timeout wording, step cap -> per-run wording).
	maxDur, clamped := clampMaxDuration(tc.DeadlineUnix, 0)
	settle := wrapupSettlement("planner", nil)
	if tc.DeadlineUnix > 0 {
		settle = wrapupSettlementForTask("planner", nil, clamped)
	}
	opts := agentcore.Options{
		Provider:        p.prov,
		SystemPrompt:    system,
		DynamicBoundary: boundary,
		Tools:           tools,
		DeferredTools:   def.Deferred,
		UnlockSet:       def.Unlock,
		PermissionMode:  permission.ModeBypass,
		EnableWebFetch:  true, // goes through the recording proxy for the audit trail; loads the proxy CA to verify MITM re-signed HTTPS certificates
		WebFetchProxy:   p.proxyAddr,
		WebFetchCACert:  p.proxyCACert,
		// Web search (optional). ddgs needs no key; brave-free needs BraveKey; tavily needs TavilyKey.
		// WebSearchProxy is an independent egress proxy (http/https/socks5), unrelated to the MITM proxy that records traffic; empty = direct.
		EnableWebSearch:       p.webSearch.Enabled,
		WebSearchBackend:      p.webSearch.Backend,
		BraveSearchAPIKey:     p.webSearch.BraveKey,
		TavilySearchAPIKey:    p.webSearch.TavilyKey,
		DeepSeekSearchBaseURL: p.webSearch.DeepSeekBaseURL,
		DeepSeekSearchAPIKey:  p.webSearch.DeepSeekAPIKey,
		DeepSeekSearchModel:   p.webSearch.DeepSeekModel,
		WebSearchProxy:        p.webSearch.Proxy,
		BashEnv:               proxyEnv(p.proxyAddr, p.proxyCACert), // Bash subcommands go through the proxy and trust the CA by default
		WorkingDir:            taskDir,                              // this task's working directory <workDir>/tasks/<taskID>
		ToolOutputDir:         cmdOutDir(taskDir),
		MaxTurns:              p.maxTurns, // 0 = unlimited (configurable in agent management)
		MaxDuration:           maxDur,     // 0=no limit; with a deadline = time left until the deadline
		Compaction:            compactionConfig(p.compactionWindow()),
		// Planning todos shared across wake-ups: keeps a serial chain alive across rounds (the session is new, the store is not).
		Todos: p.todoFor(ts.ID()),
		// Hitting [this round's] step budget -> the SDK settles: land the conclusions already worked out this round (the add_intent calls that
		// were due, the prove_goal that can be proven, the serial chain recorded with TodoWrite) instead of stopping the planning - the planner
		// is woken again repeatedly afterwards. When clamped (by the task deadline) PromptByReason is used instead (see wrapupSettlementForTask).
		Settlement:   settle,
		NonStreaming: p.nonStreaming(), // use Provider.Complete when the profile selects non-streaming
		MaxTokens:    p.maxTokens(),    // 0 = send no cap, the server default decides
	}
	if p.tx != nil { // persist raw LLM conversation; one accumulating file per task's planner
		opts.Transcript = p.tx
		opts.SessionID = fmt.Sprintf("exp%d-planner", ts.ID())
	}
	// Experimental: when on, noa takes over context compaction (archives are collected under <workDir>/noa/<SessionID>, persistent).
	noaSession := fmt.Sprintf("exp%d-planner", ts.ID())
	enableNoa(&opts, p.noaEnabledFn, p.workDir, noaSession, noaWarn(noaSession))
	// The situation (the intent just finished + the full graph) is now appended to this round's user input (see input below). user also carries
	// the instruction + cross-wake-up todos (a todo is the model's own planning note, regenerable, so user is the right place).
	// The opening line differs by "is there a concrete change this round": with a change -> point at the [actual change] block below; without
	// (heartbeat poll / hint / resume and so on) -> do not claim "the graph changed", instead prompt a review of the running intents.
	lead := "There was a concrete change just now (see [the actual change that triggered this round] below), plan the next step accordingly:"
	if len(triggers) == 0 {
		lead = "This round is a **scheduled poll (heartbeat) / no concrete change signal** wake-up - the graph may not have changed. Review the running intents along the way: use steer_work to correct one that has made no progress for a long time or has drifted, and kill_work to stop one whose direction is entirely wrong; then judge the goals and decide whether to add directions:"
		// On a heartbeat/no-change wake-up, if the whole graph has no open or running intent -> exploration has stalled (no worker running,
		// no queued direction). Tell the planner explicitly and force it to produce new directions this round, instead of just reviewing the
		// running intents and spinning for a round.
		if active, err := ts.HasActiveIntent(); err == nil && !active {
			lead = "This round is a **scheduled poll (heartbeat)** wake-up, and there is currently **no open or running intent at all** - no worker is running and no direction is queued, exploration has stalled. You **must** produce one or more new intents this round that move toward the goal and do **not** duplicate the existing intents in the graph (0 intents is not allowed); first judge from the situation below whether the goal is already met, and if not, add directions immediately:"
		}
	}
	input := lead + situational + "\n\nFrom the situation above, judge the goals. When a goal is [genuinely met] (the target result is in hand / the target finding is confirmed), mark it with prove_goal one by one. **Hard floor: as long as the goal is not met and there is currently no open or running intent (frontier_open=0 and running_intents empty), this round must produce at least one intent that moves toward the goal - with no running work to wait for and no queued direction, 0 intents means the task stalls. Only when an open/running intent is already advancing, or the goal is met, may you produce no new intent this round.**" +
		renderPlannerTodos(opts.Todos.List())
	// MaxDuration now interrupts the running tool at the wall-clock limit and settles in place (on the live ctx), so a stuck round no longer
	// bypasses settlement and no external hard ctx backstop is needed. ctx only carries pause / kill / shutdown.
	_, _, err = captureRun(ctx, opts, input,
		func(r db.Activity) {
			if emit != nil {
				r.Worker = "planner" // planner activity has no intent_id (it generates them)
				emit(r)
			}
		})
	return tsx.GoalMet, tsx.Reason, err
}
