# ARTEX-HYDRA

- url: https://github.com/j5tv9nfzhn-hue/ARTEX-HYDRA
- slug: `hydra` (git remote, kept)
- first reviewed: 2026-11-05
- branch: `main` @ `6d4175b3`
- tags: 28, latest `v2.5.0.6`

## Relationship

- Forked from the same upstream base as us (Autumn-27 / jiwoochris global
  edition). No merge-base with our history: our hashes were rewritten by
  filter-repo, so relationship is measured content-wise.
- 1206 commits on `main`; ~700 by the fork author (`j5tv9nfzhn-hue`) on top of the
  imported upstream history.
- Diff vs our `dev`: 1181 files, +152k / -72k lines. Changed most in `web` (254),
  `server` (248), `agent` (144), `db` (137), `skills` (100).

## Direction

- Simplified-Chinese-only edition: UI, reports, summaries, chat and backend
  messages are uniformly zh; the repo states no other UI language is provided.
  ~126 `feat(i18n)` commits replace the locale system instead of extending it.
- New `mcpserver/` package (builtin/guard/http/probe) — an in-process MCP server,
  we do not have this.
- Skills overhaul: removed "fix" class skill content, converged the whole skills
  tree to discover-only / exploit-only / report-only wording; SCA structured
  fields moved onto `report_finding`.
- Prompt-side: operation-constraint injection; delivery wording converged to
  discover/exploit/report only.
- Upstream-synced fixes worth noting: sidequestion terminal checkpoint on early
  consumer exit; notification test fixture isolation for flaky stray events.
- Release engineering: own release workflow, README rewritten with install
  tutorial + 12 screenshots, v2.5.x tag line.

## Detailed differences (deep review 2026-11-05)

### Architecture: planner/worker replaced by a coordinator

The central change. Our runtime (and the global base) is planner + N workers with
intents. HYDRA replaced it with a single adaptive coordinator:
`agent/coordinator.go`, `coord_assembly.go`, `coord_switch.go`, `coord_tools.go`,
wired into the server via `server/engine_coord*.go` (engine bridge + wire).
Commit line: "adaptive agent rework — capability routing + clue cache + induced
toolbox" (`0cafa6a7`) and "align agent runtime with strix; drop planner/worker
leftovers" (`99b6a076`). Capability routing is exposed as a service
(`server/capability.go`, `capability_http.go`). The goal graph stays
(`agent/goals.go`), so goal/prove semantics survive; intent dispatch does not.
This is the same "Strix" lineage appearing in several commits — HYDRA tracks an
external reference implementation called Strix and ports its behaviours.

### LLM reliability layer

`agent/provider_retry.go` + `server/llmretry.go`: failure classification in one
place, retry vs endpoint-switch decided by the classification; retry gradient
2 s doubling, capped at 90 s, 5 attempts default (operator override wins). Our
llmpool has a simpler chain failover; their classification-first approach is the
more engineered one. Cherry-pick candidate.

### Findings: dedup + CVSS + server-side verification

- `server/dedupe.go` + a settings-page dedup component, wired to the backend —
  cross-finding duplicate judgement.
- `agent/finding_cvss.go` — CVSS v3.1 score calibration, ported from Strix, in
  the finding registration path.
- `server/finding_verify.go` — server-side finding verification step. Overlaps
  conceptually with our `report_finding` verification gate, but implemented as a
  server workflow rather than a tool-input requirement. Worth a design
  comparison before taking either side further.

### Storage and data layer

`db/` gained ~54 files. Highlights:
- `dialect_*` family (dml/dsl/arrays/companies/intercept) + `archive_restore_sqlite.go`
  — a database dialect abstraction with SQLite archive/restore. Driven by the
  desktop product (see below), where a bundled SQLite makes sense.
- `db/credential_crypto.go` + `server/credential_key.go` — encrypted credential
  storage at rest with key management. We store credentials unredacted by
  decision (ADR-0001); their take is encryption rather than procedure. Not
  conflicting, but a different threat-model answer.
- `db/clue_cache.go`, `db/checkpoints.go`, `db/compaction_state.go`,
  `db/activity_retention.go` — coordinator clue cache, run checkpoints,
  compaction state across restarts, activity retention policy.

### Agent context management

`agent/context_budget.go`, `context_compaction.go`, `compaction_layers.go`,
`convbudget.go`, `ctxwindow.go`, `history_prune.go` — layered context budgeting
and pruning. Our compaction is single-shot; theirs is tiered with persisted
state. Candidate for study, not blind port (depends on their coordinator).

### Product surface

- Wails V2 desktop shell (`artex-hydra`), installer, tray, window defaults; the
  repo here carries `cmd/artex-launcher`; the desktop body lives in a separate
  repo ("dual release"). Local-first SQLite + data-migration settings to move
  data between desktop and web (`feat(migrate)`).
- CLI user entry with install-directory guard and live web-config sharing
  (`feat(cli)`); a terminal HARNESS replaced the web chat page
  (`ecb3e747` removed the web conversation page — we still have it).
- New web pages: notification center, sidebar customization, asset/command/LLM
  record detail pages, handoff dialog, software-re tab, timeline tab, task
  session page, traffic detail, external-intercept detail, LAN page, MCP detail.
- Theme system "纸本/编年" (editorial paper skin) with design tokens
  (~20 `refactor(theme)` commits).
- `boot/` package: console/job-object handling (Windows service semantics),
  password reset, install guard — desktop/runtime bootstrap.
- Versioning converged to git tags; desktop and web share one four-part version.

### MCP server in-process

`mcpserver/` (11 files: builtin/guard/http/probe) — an in-process MCP server
with its own guard. We connect to external MCP servers; they also host one.
No conflict with our tree layout; clean candidate.

### Skills: wholesale replacement

Their `skills/` tree (~80 packs) is entirely third-party open-source packs
(commit `cdc43a49` deleted all hand-written skills; `7a5888fa` re-fetches packs
on compaction/drift/cross-run). Ours is 4 curated packs mounted into the
container. Different philosophy: breadth-by-import vs curated-minimal. Their
packs embed via `skills/embed.go` (packs compiled into the binary).

### What they do NOT have (our side)

- Demo mode (screen-share masking) — absent entirely.
- Our ru locale and the en-literal catalog model — they went zh-only single
  locale (`web/messages/zh.json` only).
- Grouped `internal/`/`llm/` layout, short module name — cosmetic, irrelevant.
- Our measurement-discipline prompt gates and the credential ADR policy.

### What we do NOT have (their side), ranked by value

1. Coordinator + capability routing (big; only with a design decision).
2. LLM failure classification / retry gradient (`provider_retry.go`).
3. Finding dedup + CVSS calibration (self-contained, `finding_cvss.go` +
   `server/dedupe.go`).
4. Server-side finding verification (`finding_verify.go`) — compare designs.
5. Encrypted credential storage (`credential_crypto.go`) — if our threat model
   changes per ADR-0001's revisit clause.
6. Context budget layers (depends on coordinator).
7. `mcpserver/` in-process MCP server.
8. Activity retention / WAL health / storage observability pages.

## Assessment

- Merge as a whole: no. The zh-only direction directly contradicts our en/ru
  localization model; the i18n diff is the bulk of the changeset and is not
  portable.
- Cherry-pick candidates (by value, small and self-contained):
  - `mcpserver/` package — new capability, no conflict with our tree layout.
  - sidequestion terminal-state checkpoint fix (consumer early exit).
  - skills wording cleanup (discover/exploit/report only) — same policy we follow
    for measurement discipline, but needs rewording to English on our side.
- Maintenance signal: high velocity, active tags, CI workflows present.
  A serious, actively maintained fork.

## Decision

`watch`. Re-review on new minor tags (v2.6+) or on request. Cherry-pick only the
items listed above, each as its own task with our regression tests; highest-value
first: `provider_retry.go`, `finding_cvss.go` + dedup, `mcpserver/`. The
coordinator architecture and the zh-only product surface are tracked for design
awareness, not for porting.
