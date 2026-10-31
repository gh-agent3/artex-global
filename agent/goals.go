package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Autumn-27/artex/db"
	"github.com/Autumn-27/norma/agentcore"
	"github.com/Autumn-27/norma/llm"
	acperm "github.com/Autumn-27/norma/permission"
	actool "github.com/Autumn-27/norma/tool"
	"github.com/Autumn-27/norma/transcript"
)

// goalsDefaultTmpl is the built-in EDITABLE body (section [A]) of the goals-decomposer
// prompt, seeded into agent_prompts. No template vars are used today.
// prompt, seeded into agent_prompts. No template vars are used today.
const goalsDefaultTmpl = `You are a penetration-testing goal decomposer. Your job is to identify from the user input the **results that must ultimately be achieved**, not to plan attack steps.

**Step one (do this before decomposing goals): extract the operating constraints**
From the task goal / task description, identify the operator's explicit rules about [what may and may not be done], and register each one with set_constraints (if the goal and description contain no operating constraints, skip this step):
- type=deny: forbidden operations (e.g. "do not scan ports", "no writes or deletes against production", "no brute force", "stay away from subdomain X").
- type=allow: explicitly allowed or restricted scope (e.g. "passive reconnaissance only", "this domain only").
- A constraint is not a goal and not an attack step: it constrains the boundary of the operations.
- **Every constraint must be [self-contained and pin the concrete target]**: replace referring expressions such as "the current target / current port / current IP / current domain / this site" with the **concrete value** from the goal or description. Constraints are injected into the execution prompt on their own, and outside this context a referring expression cannot be resolved.
  Example: if the goal is https://abc.example.net, write "only abc.example.net may be tested" rather than "only the current target may be tested"; "test only port 443 of the target, do not scan other ports" rather than "test only the current port". If the text only says "the current target" but the goal already names the address, put the address in.
- **Register only constraints that are [explicitly written or emphasised] in the goal/description; never invent them**; when the type is unclear use deny (the more conservative choice).
- If the goal/description genuinely contains no operating constraint, do **not** call set_constraints.
Once the constraints are registered (if any), do the goal decomposition below.

**A goal = a final, deliverable and verifiable result**

**What is not a goal (never list these as sub-goals)**:
- information gathering, reconnaissance, endpoint scanning
- vulnerability analysis and verification steps
- attack steps and exploitation techniques
- result-verification steps

**Decomposition rules**:
- The user described exactly one final goal -> output one
- Several **mutually independent** final deliverables -> list each separately
- Tag vulnclass when the goal maps to a clear vulnerability class; leave it empty for information-gathering or business-logic goals
- Never invent goals the user did not mention

Call set_goals to submit the result.`

// goalsScopeTail is the code-owned tail appended after the editable goals body
// WHEN an asset store + task context are available. It teaches the decomposer to
// also lift the explicit asset scope out of the goal/description and register it
// via add_task_scope. Kept in code (not the DB-editable body) so it always applies
// on released DBs and can't be edited away — same pattern as the trafficTool tail.
const goalsScopeTail = `

**Extra duty: register the test asset scope**
Besides decomposing goals, identify in the task goal / task description the **explicitly given test asset scope** and register it with add_task_scope (this task's authorisation boundary and the denominator of asset test coverage). **Minimum-scope principle: register only the single target the user explicitly named, and never widen it on your own.**
- When the goal is a URL or a hostname-bearing address (e.g. https://xxx.example.com/path, app.example.com) -> take its **full hostname**, kind=subdomain, value=the full hostname.
  Example: goal https://a1b2c3.lab.example.net/path -> kind=subdomain, value=a1b2c3.lab.example.net (**not** example.net).
  **Never** shorten a hostname that has a subdomain to its root domain - registering example.com after seeing xxx.example.com widens the scope beyond the user's target and violates the minimum-scope principle.
- Only when the user gave a **bare root domain with no subdomain at all** (e.g. literally example.com), or explicitly said "the whole site / all subdomains / the entire domain" -> use kind=root_domain, value=example.com.
- A bare IP or a CIDR block -> kind=ip / cidr, value=the IP or CIDR.
- Do **not** register a company scope (company): the task was just created and the asset system usually does not know that company yet, so registration would fail; company-level scope is handled in the later planning phase.
Other rules:
- Register only what is **explicitly written** in the goal/description; never invent or infer a domain/IP that was not mentioned.
- In reason, state briefly which sentence the scope comes from, so it can be audited.
- If the goal/description carries no explicit asset scope, do **not** call add_task_scope.
First register the scope with add_task_scope (if any), then call set_goals to submit the goals.`

// goalsSystem assembles the goals-decomposer system prompt: the rendered body
// [A] (DB-overridable), the code-owned scope-extraction tail when add_task_scope
// is wired (withScope), and the code-owned Korean output-language tail [C] last —
// mirroring chatSystem/plannerSystem so a DB-edited body can never drop the tail.
// DecomposeGoalsWithProvider and the localization test share this one assembly, so
// the langDirective tail can't drift between runtime and test. EngagementDescription
// is intentionally left empty: the task description rides in the user message, not
// the {{.EngagementDescription}} var (see DecomposeGoalsWithProvider).
func goalsSystem(dataDir string, withScope bool) string {
	sys := renderSystem("goals", goalsDefaultTmpl, GoalsVars{DataDir: dataDir, Now: nowStr()})
	if withScope {
		sys += goalsScopeTail
	}
	return sys + langDirective(resolveOutputLang())
}

// GoalSpec is one decomposed objective.
type GoalSpec struct {
	Text      string `json:"text"`
	VulnClass string `json:"vulnclass,omitempty"`
}

// DecomposeGoals asks the LLM to break a pentest task goal into discrete,
// independently-verifiable objectives (each becomes a goal node). Returns nil if
// no provider is configured or the call yields nothing — the caller then falls
// back to a rule-based split so goal nodes always exist.
//
// prov is supplied by the caller (rather than built here from a Config) so goal
// decomposition rides the SAME provider instance as the rest of the engine — it
// shares the rate limiter, gets recorded by llmrec, and participates in LLM
// failover instead of quietly bypassing all three.
//
// desc is the task's free-text description (background: target scope / flag count / engagement notes).
// It is fed alongside the goal so the decomposer no longer splits blind — the
// prompt still forbids inventing anything the two texts don't state.
//
// emit, when non-nil, receives every LLM step (thinking/tool_use/result) with
// Worker="planner" so the round-0 goal-decomposition activity is visible in the UI.
//
// as + taskID, when non-nil/positive, wire the add_task_scope tool so the
// decomposer can register the explicit asset scope it extracts from the goal.
//
// ts is the task's exploration store: set_goals writes the decomposed goal nodes
// straight into it (the same managed tool the main agent uses to add goals at
// runtime). The returned specs are read back from the store so callers can emit
// per-goal activity and detect the "LLM produced nothing" case for their fallback.
func DecomposeGoals(ctx context.Context, prov llm.Provider, dataDir, goalText, desc string, as *db.AssetStore, ts *db.ExplorationStore, taskID int64, emit func(db.Activity)) []GoalSpec {
	if prov == nil {
		return nil
	}
	return DecomposeGoalsWithProvider(ctx, prov, dataDir, goalText, desc, as, ts, taskID, false, 0, emit)
}

// DecomposeGoalsWithProvider is the task-runtime variant used when a task has an
// ordered provider chain. It preserves the same tools and write behavior while
// letting the caller own provider selection/failover. maxTokens is the profile's
// per-reply output cap (0 = send none).
func DecomposeGoalsWithProvider(ctx context.Context, prov llm.Provider, dataDir, goalText, desc string, as *db.AssetStore, ts *db.ExplorationStore, taskID int64, nonStreaming bool, maxTokens int, emit func(db.Activity)) []GoalSpec {
	if prov == nil {
		return nil
	}
	// Goal decomposition is a single call that attaches no transcript store, so agentcore does not put
	// a session id on the ctx (it only does so when a writer exists, see agentcore.Prompt). Gateways
	// that key prompt caching or sticky routing on a session-id header (opencode zen answers 400
	// MissingSessionID without x-opencode-session) read exactly that value - leave it unset and you
	// get "chat works, decomposition returns 400". Attach a stable id explicitly: every decomposition
	// request of the same exploration shares it (good for cache hits), and the name does not clash with
	// planner/worker so llmrec.parseSession attributes it correctly.
	if ts != nil {
		ctx = transcript.WithSessionID(ctx, fmt.Sprintf("exp%d-goals", ts.ID()))
	}
	// worker="goals" tags the goal nodes' provenance; ts/taskID let set_goals link
	// each goal under the task root. This is the catalog's real set_goals tool, so a
	// web-edited description/schema on it applies here too.
	tsx := &ToolSet{as: as, ts: ts, taskID: taskID, worker: "goals"}
	// Wire add_task_scope only when we have a real asset store + task to write to.
	// goalsSystem appends the scope-extraction tail in lockstep (withScope) so the
	// prompt never asks for a tool that isn't present, and it owns the output-language
	// tail last so a DB-edited body can't drop it. Description rides in the user
	// message, NOT the {{.EngagementDescription}} var, so a prompt can't inject it twice.
	withScope := as != nil && taskID > 0
	sys := goalsSystem(dataDir, withScope)
	// set_constraints is always available (it does not depend on the asset store): the body already
	// contains the "extract the constraints, then decompose" step (wording editable on the agent page),
	// so here we only need to wire the tool in.
	tools := []actool.CoreTool{tsx.setGoals(), tsx.setConstraints()}
	if withScope {
		tools = append(tools, tsx.addTaskScope())
	}
	userMsg := "Task goal:\n" + goalText
	if d := strings.TrimSpace(desc); d != "" {
		userMsg += "\n\nTask description (background; may contain the target scope / flag count / engagement notes; reference only - never invent anything it does not state):\n" + d
	}
	// Use captureRun so every LLM step is emitted as an activity record (visible in
	// the plan tab under the round-0 marker). Falls back gracefully when emit is nil.
	captureEmit := func(r db.Activity) {
		if emit != nil {
			r.Worker = "planner"
			emit(r)
		}
	}
	captureRun(ctx, agentcore.Options{
		Provider:               prov,
		SystemPrompt:           []string{sys},
		Tools:                  tools,
		PermissionMode:         acperm.ModeBypass,
		DisableBackgroundTasks: true,
		// Three steps (extract constraints -> register scope -> decompose goals) each need one tool call,
		// so give enough turns to avoid missing set_goals before settlement.
		MaxTurns:     8,
		NonStreaming: nonStreaming, // use Provider.Complete when this profile selects non-streaming
		MaxTokens:    maxTokens,    // 0 = send no cap, the server default decides
	}, userMsg, captureEmit)
	// set_goals persisted the goals directly; read them back so the caller sees what
	// was written (empty slice ⇒ the LLM produced nothing ⇒ caller falls back).
	if ts == nil {
		return nil
	}
	nodes, _ := ts.ListByKind(db.KindGoal, 10000)
	var out []GoalSpec
	for _, n := range nodes {
		var p struct {
			Text      string `json:"text"`
			VulnClass string `json:"vulnclass"`
		}
		_ = json.Unmarshal(n.Payload, &p)
		if strings.TrimSpace(p.Text) != "" {
			out = append(out, GoalSpec{Text: p.Text, VulnClass: p.VulnClass})
		}
	}
	return out
}
