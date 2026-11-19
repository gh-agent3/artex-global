# ADR-0003: Standing fork-harvest process — collect the best from ARTEX forks regularly

Status: accepted
Date: 2026-11-19

## Context

The original ARTEX (Autumn-27/ARTEX) was taken down in October 2026, and the
framework now lives on through community forks — each fixing different things
and adding different features. Google Threat Intelligence Group's report
"ARTEX and Friends: Enter Agentic Offensive Frameworks" (2026-10-31) confirms
both the proliferation and the wild abuse, and lists the fork ecosystem
explicitly (ScopeWeaver, ARTEX PRO among them).

Our distribution (ACP Edition) diverged too far from every fork for merges,
but every fork is still a source of spot improvements. Reviewing forks once
and porting a batch leaves the value on the table: forks keep moving. The
competitive edge of this distribution is that we collect the best from all
of them, continuously — so harvesting is a standing process, not a one-off
task.

## Decision

A recurring fork-harvest cycle, run on a fixed cadence, with manual spot
ports only (no merges):

1. **Watchlist** — the tracked forks live as git remotes in this repo and in
   `docs/forks/`. The canonical ranking source is
   https://www.artex.sh/forks/best (community leaderboard); new entries there
   get added to the watchlist.
2. **Cadence** — each cycle: fetch all watchlist remotes, diff against the
   previous fetch, skim new commits (subjects + fix/security labels first).
3. **Triage** — every candidate is classified: PORT (spot port now), BACKLOG
   (valuable, needs design or waits its turn), REJECT (recorded with a one-
   line reason — e.g. jaykwok's removal of the .gov/.edu blocklist), N/A
   (already covered by a different mechanism in our tree).
4. **Port** — manual, minimal, test-covered. Each port lands as its own
   commit with the source commit hash in the message body reference and the
   port recorded in `docs/backlog.md` / `CHANGELOG.md` with repo + commit.
5. **Verify** — full check suite before release: `go build ./...`, PG + SQLite
   db suites against the pre-existing-failure baseline, server suite vs the
   stashed baseline, agent/llm/internal packages, `tsc`, `check:i18n`,
   doc-links. Then image build, deploy, live smoke, push, tag rollover.
6. **Record** — the cycle outcome is written into `CHANGELOG.md` and the
   per-fork review notes in `docs/forks/`, so the next cycle diffs from a
   known state.

## What we already harvested (the first cycle, 2026-11)

See `CHANGELOG.md` for the full per-commit attribution with repo links. In
short:

- **failsafe-security/artex** — self-update compiled out, pinned playwright-mcp.
- **BaaRai/ARTEX** — task run budgets, prompt-injection framing line.
- **RuoJi6/ARTEX** — MCP ingress source allowlist.
- **wpsec/ARTEX-Pro** — CSRF write-path hardening, os.Root workspace
  confinement, skill-upload default authorization, visibility opt-out
  durability, side-question capture completion fix, dependency bumps.
- **AntiFox-AI/antifox** — kill-chain reproduction-steps section (minimal port).
- **google-research/harness2** — harness lessons (already ADR-0002).
- **jaykwok/ARTEX** — reviewed, rejected (blocklist removal).

## Considered and not taken

- Merging any fork wholesale — our tree diverged too far; merges would
  re-import what we deliberately removed.
- Automated cherry-pick bots — ports need adversarial judgment (what fits our
  threat model, what our tree already covers differently); a bot cannot make
  the REJECT/N-A calls.
- Subtree/submodule tracking — same reason; spot ports keep the tree ours.

## Consequences

- Every release notes which forks fed it and which commits were ported.
- `docs/backlog.md` carries the live triage state between cycles.
- The watchlist remotes make the next cycle a `git fetch --all` away.
