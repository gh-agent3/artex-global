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
items listed above, each as its own task with our regression tests.
