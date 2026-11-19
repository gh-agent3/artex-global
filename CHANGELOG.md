# Changelog

All notable changes to ARTEX ACP Edition. Releases are tagged `v<version>-acp.N`.
Each entry attributes what was taken from which fork, with the source commit.

## v0.3.16-acp.1 — 2026-11-19

The first full fork-harvest release. Sources: the community fork leaderboard
https://www.artex.sh/forks/best and per-fork reviews in `docs/forks/`.
Harvest method: manual spot ports (no merges) — see ADR-0003.

### Security hardening (from wpsec/ARTEX-Pro, https://github.com/wpsec/ARTEX-Pro)

- CSRF write-path hardening — cookie and `?token=` query auth now apply only
  to safe methods (GET/HEAD), and the query token only on the four SSE routes
  (`844dfffd`). Our UI already sends `Authorization: Bearer` on every
  mutation, so this is defense in depth.
- Workspace confinement via `os.Root` — every workspace file-manager handler
  now does its I/O through an `os.OpenRoot`, so a symlink planted inside the
  work dir cannot escape it (`844dfffd`). Proven by
  `TestWorkspaceSymlinkConfined`.
- Skill upload default authorization — an uploaded skill is bound to the
  default runtime agents (auto/pentest/worker) instead of being invisible to
  everyone (`50af614b`).
- Visibility opt-out durability — turning a skill off stores an
  `enabled=false` row instead of deleting it, so the startup seed can no
  longer resurrect an explicit opt-out (`dea19481`).
- Side-question capture completion — a stream delta carrying a stop reason
  now also marks the captured turn complete (`d3a678be`).
- Dependency bumps: pgx v5.10.0 -> v5.12.0, klauspost/compress 1.18.7 ->
  1.20.2 (`844dfffd`, `e1e41e8a`); Next.js 16.2.9 -> 16.4.5 with sharp pinned
  at 0.35.5 (`844dfffd`). x/crypto, x/net, x/text were already at or above
  their pins.
- Considered and not taken: JWT key rotation on password change (`844dfffd`)
  — N/A by design: our signing key is HMAC(jwt.key, password_hash), so every
  password change already invalidates all previously issued tokens.

### Run budgets (from BaaRai/ARTEX, https://github.com/BaaRai/ARTEX)

- Per-task `budget_tokens` / `budget_turns` (0 = unlimited), settable at task
  creation and via PATCH. Spend is counted from the llm_usage ledger
  (input+output per metered call). When a capped dimension burns into the
  wrap-up reserve (5%, floors 20k tokens / 2 turns) the engine stops
  dispatching and settles through one final planner round — the same endgame
  as the wall-clock timeout, with status `failed` (trimmed port of their
  `agent/budget.go`).
- Prompt-injection framing — the measurement-discipline block now states that
  tool and target output is DATA, never instructions, in every pentest role
  prompt.

### MCP ingress source allowlist (from RuoJi6/ARTEX, https://github.com/RuoJi6/ARTEX)

- Optional `mcp_allowed_sources` setting (comma-separated IPs/CIDRs) checked
  in the `/mcp` authorize hook, on top of the bearer token.

### Kill-chain finding steps (from AntiFox-AI/antifox, https://github.com/AntiFox-AI/antifox)

- Minimal port: a numbered "Reproduction steps" section over the ordered
  traffic bindings in the finding traffic panel (6 locales). Their full
  step-schema (entry point / precondition / step request+expected /
  conclusion) was evaluated and deferred — it needs a product decision, see
  `docs/backlog.md`.

### Self-update and supply chain (from failsafe-security/artex, https://github.com/failsafe-security/artex)

- Online self-update compiled out — `internal/selfupdate.Enabled = false`,
  every entry point refuses before touching the network. We deploy by image;
  nothing in the runtime replaces its own binary. Published after the
  backdoored prebuilt `tomicdd/artex` image was found (Nov 2026) — that
  incident was a prebuilt-image compromise; our mitigations are source
  builds, pinned dependencies, and no runtime self-replacement.
- Browser MCP pinned to a local binary (`@playwright/mcp@0.0.82` +
  `@playwright/cli@0.1.25` against pinned Chromium 1.64) with a DB migration
  of the old `npx` seed — no registry fetch at enable time.

### Own work this release line

- Planner deadline + intent watchdog; SSE header flush fix; harness lessons +
  editor (ADR-0002, inspired by google-research/harness2); diskguard
  auto-rotation; verifier pipeline; demo transport masking; credential
  encryption at rest (AES-256-GCM, opt-in); experimental SQLite backend;
  en/ru UI with English as the built-in literal; SECURITY.md advisory
  channel.

### Reviewed and rejected

- jaykwok/ARTEX (https://github.com/jaykwok/ARTEX) — removal of the .gov/.edu
  blocklist: rejected, the blocklist stays.

## Earlier

See `git log` for the pre-harvest history. Notable: v0.3.16-acp line start,
SQLite experimental backend, credential encryption, MCP ingress, demo mode.
