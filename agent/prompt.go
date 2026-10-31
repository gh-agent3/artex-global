package agent

import (
	"bytes"
	"fmt"
	"text/template"
	"time"
)

// PromptOverride, if set, returns the stored system-prompt template for an agent
// key and whether one exists. The server wires it to the PG agent_prompts table.
// When nil or no override exists, agents use their built-in default prompt — so
// behavior is identical until a user edits a prompt in the UI.
var PromptOverride func(agentKey string) (string, bool)

// Prompt-variable structs — fields mirror each agent's catalog (docs §5a) so a
// user template referencing a catalog variable renders; referencing anything else
// fails template execution and falls back to the built-in default.
type PlannerVars struct{ Goal, Scope, AssetSummary, DataDir, Now string }
type WorkerVars struct{ ProxyAddr, WorkerName, DataDir, Now string }
type MainVars struct{ Goal, AssetSummary, FindingsSummary, DataDir, Now string }
type GoalsVars struct{ EngagementDescription, DataDir, Now string }

// nowStr is the server-local wall-clock string exposed as the universal {{.Now}}
// prompt variable. renderSystem runs on every agent turn/round, so this is fresh
// each run — a prompt can subtract it from a fixed start stamp to reason about
// elapsed time (e.g. a timed benchmark's "last N hours" window).
func nowStr() string { return time.Now().Format("2006-01-02 15:04:05 MST") }

// renderSystem returns the rendered system-prompt BODY (section [A]) for agentKey.
// Precedence: the DB-stored template (if any) over the built-in default template
// (def). BOTH are Go templates now — the built-in default is seeded into the DB
// verbatim, so the two paths render identically until a user edits the prompt.
// Rendering always runs (def used to be pre-substituted plain text; it is now a
// {{.Var}} template like the DB one). On any render error we fall back to the
// default template, then to the raw default string — an agent never starts with a
// half-rendered prompt. Callers append the code-owned tail (trafficTool / artifact
// output rules) AFTER this, so those cannot be edited away via the DB body.
func renderSystem(agentKey, def string, vars any) string {
	tmpl := def
	if PromptOverride != nil {
		if t, ok := PromptOverride(agentKey); ok && t != "" {
			tmpl = t
		}
	}
	if out, err := renderTmpl(tmpl, vars); err == nil {
		return out
	}
	// DB template broke (e.g. references an out-of-catalog var) → code default.
	if out, err := renderTmpl(def, vars); err == nil {
		return out
	}
	return def
}

// langDirective is the output-language tail: a code-owned segment
// appended AFTER the rendered body and the artifact/traffic tails on every
// user-facing agent role, so a DB-edited prompt body can never drop it — the same
// guarantee artifactSpec gives. It does NOT translate the agent "brain": the
// benchmarked reasoning body (section [A]) stays verbatim. It only constrains
// the LANGUAGE of what the agent SHOWS to the user. Written in Chinese so it stays
// in the body's language (keeping the model's reasoning register stable) while
// forcing the chosen display language — this is the localization approach:
// preserve behavior, localize the surface the user reads. Raw technical strings
// (commands, payloads, code, URLs, log/response excerpts) are explicitly kept
// verbatim so evidence and reproduction steps are not mangled by translation.
//
// Two anti-drift clauses were added after the end-to-end run (L1): live models
// leaked (1) Chinese into the planner's situation summary — mirroring the Chinese
// brain body (section [A]) - and (2) the target app/evidence language into
// report_finding's structured fields. The directive names the planner situation
// summary as a user-facing field and forbids mirroring BOTH the Chinese
// instruction language AND the target/material language in the display fields.
//
// The display language is a parameter (code = "en"/"ko"/"zh"/"es"): the directive
// stays in Chinese but the "write in <language>" token is swapped per target. When
// the display language IS Chinese ("zh"), Chinese output is the goal, so the
// "never emit Chinese to the user" clause is replaced by one that simply affirms
// Chinese display and still forbids mirroring the target/material language.
func langDirective(code string) string {
	phrase := langPhrase[code]
	bare := langBare[code]
	if phrase == "" || bare == "" {
		phrase, bare = langPhrase[defaultOutputLang], langBare[defaultOutputLang]
	}
	if code == "zh" {
		// 표시 언어 자체가 중국어이므로 "중국어를 사용자에게 내보내지 말라"는 절은 없다.
		// 대신 중국어 표시를 확정하고, 대상/자료 언어 미러링 금지와 원문 보존은 유지한다.
		return fmt.Sprintf("\n\n**Output language rules (localisation; highest priority, cannot be overridden by the prompt body)**: every natural-language string [shown to the user] must be written in [%s] - including record_fact summary/detail, report_finding title/description/conclusion/remediation, the planner's situation summary, the final one-sentence summary, and replies in chat. But [commands, payloads, code, file paths, URLs, parameter names, and the literal fragments of logs/requests/responses] must be kept [verbatim] - never translated or reworded (command lines and outputs inside evidence especially must be copied as-is so the finding stays reproducible). **Even when the target system, its pages, evidence, logs or any reference material are written in English or another language, the user-facing natural-language fields (title/description/conclusion/remediation/summary/situation summary) must still be written in %s - do not mirror or copy the language of the target or the material for those fields; only the literal technical fragments listed above stay as they are.** In one sentence: internal reasoning is unconstrained, but anything that reaches text the user can see must be entirely in %s (technical fragments aside).", phrase, bare, bare)
	}
	return fmt.Sprintf("\n\n**Output language rules (localisation; highest priority, cannot be overridden by the prompt body)**: every natural-language string [shown to the user] must be written in [%s] - including record_fact summary/detail, report_finding title/description/conclusion/remediation, the planner's situation summary, the final one-sentence summary, and replies in chat. But [commands, payloads, code, file paths, URLs, parameter names, and the literal fragments of logs/requests/responses] must be kept [verbatim] - never translated or reworded (command lines and outputs inside evidence especially must be copied as-is so the finding stays reproducible). **Even when the system/role instructions above are themselves written in Chinese, never emit Chinese to the user - the only user-facing display language is %s, and no Chinese sentence may appear in user-visible text.** **Even when the target system, its pages, evidence, logs or any reference material are written in English, Chinese or another language, the user-facing natural-language fields (title/description/conclusion/remediation/summary/situation summary) must still be written in %s - do not mirror or copy the language of the target or the material for those fields; only the literal technical fragments listed above stay as they are.** **Your analysis/planning/thinking may be in Chinese, but that is [invisible internal reasoning] and must never be emitted as body text: a visible reply starts in %s from the first character - do not prefix it with Chinese reasoning, explanation or an \"I will first...\" preamble; even clarifications, missing parameters or \"cannot continue\" notes are written directly in %s.** In one sentence: internal reasoning is unconstrained, but anything that reaches text the user can see must be entirely in %s (technical fragments aside).", phrase, bare, bare, bare, bare, bare)
}

func renderTmpl(tmpl string, vars any) (string, error) {
	t, err := template.New("p").Option("missingkey=error").Parse(tmpl)
	if err != nil {
		return "", err
	}
	var b bytes.Buffer
	if err := t.Execute(&b, vars); err != nil {
		return "", err
	}
	return b.String(), nil
}
