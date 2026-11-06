# ADR-0002: Harness lessons (harnessing-level recursion) inspired by Google Harness²

Status: accepted
Date: 2026-11-06

## Source

- Project: https://github.com/google-research/harness2 (Apache-2.0), paper
  arXiv:2610.14224 "Recursive Agent Harnessing for an Open World".
- What we took: the **experience-library** idea and its discipline — after a
  task, a reflector derives at most ONE durable lesson; a lesson is
  `{cue, lesson, evidence, confidence}` where the cue names the SHAPE of the
  situation (a transferable pattern, not the task's facts); lessons are stored
  per-domain and injected whole into later tasks (never summarized).
- What we did NOT take: the Harness² runtime, benchmarks, Python code, the
  Propose–Probe–Compose task-level recursion with re-executions (probes), and
  their harness-tree editor. We adopted the harnessing-LEVEL recursion only,
  reimplemented in our Go stack on the verifier agent's machinery.

## Why

The curiaforum.com engagement produced the same categories of defects we had
already fixed by hand elsewhere: probe scripts crashing on empty results,
claims refuted by source, limiter conclusions drawn from the wrong bucket.
That knowledge lived in ADRs, prompt blocks and the operator's head — nothing
carried it into the next engagement automatically. Harness²'s experience
library is exactly the mechanism for that: small, evidence-bound, transferable.

## What ARTEX now does

1. When a task reaches a terminal state, a scanner claims it (one reflection
   per task, `harness_reflections`) and runs a seeded read-only "reflector"
   conversation agent.
2. The reflector reads a frozen run summary (activity/tool-error counts,
   LLM errors, findings by verification verdict, refuted reasons) and either
   records exactly one lesson (`harness_lessons`: cue/lesson/evidence/
   confidence, domain-tagged from task_scope) or ends without one (NO_ENTRY
   is a valid outcome — invented lessons poison the library).
3. Every planner/worker run gets a short "Lessons from earlier engagements"
   tail: global lessons plus lessons whose domain matches the task's
   root_domain scopes, capped at six, ordered by confidence then recency.
   The block is guidance, not orders.
4. UI: a Lessons page (`/function/lessons`) and a dashboard card; API
   `GET /api/harness/lessons`.

## Consequences

- One extra LLM conversation per finished task (bounded, on the default
  profile). Reflections older than 2h left `running` are sealed `no_entry`
  by a reaper; the unique task_id means a crashed reflection is not retried.
- Lessons are operator-visible and deletable; a wrong lesson can be removed
  from `harness_lessons` directly.
- Future option (deliberately not built): task-level propose–probe–compose —
  re-running tasks with edited harnesses is a large budget decision, not a
  default. Revisit if lesson injection alone stops paying off.

## Phase 1 (2026-11-07): harness editor — proposals with operator approval

A seeded "harness editor" agent (same conversation machinery as the
reflector/verifier) periodically takes the adopted lessons that have no
proposal yet and turns them into AT MOST ONE concrete prompt-template edit
(`harness_proposals`: agent_key, summary, rationale, full template, lesson
links). Validation bar = the same `validateTemplate` as a manual UI edit.
Nothing applies automatically: the operator decides on the Lessons page →
Proposals tab. Apply goes through `SavePrompt` (version bump +
`current_prompt_id` switch), so roll-back is the prompt versions page; the
applied version is recorded on the proposal. One batch at a time: the trigger
is gated on zero pending proposals.

## Phase 2 (recorded, deliberately not started)

A/B probes on future tasks: assign a prompt version per task, compare
structured signals (tool errors, refuted findings, goal-met) across versions,
auto-adopt/revert. Deferred because at the current task volume statistical
contrast accrues too slowly to beat the operator's judgment; also requires
prompt-version tracking on tasks. Revisit when weekly task volume justifies it
or when a disposable lab target exists (see the harness² discussion in
docs/backlog.md).
