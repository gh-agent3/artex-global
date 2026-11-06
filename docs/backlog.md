# Backlog

Things decided but not yet implemented. Newest on top within a section.

## Ported from external research / forks

1. **[L] Harness lessons** — DONE (2026-11-06, from Google Harness²'s experience
   library, NOT their runtime — see docs/adr/0002-harness-lessons.md). Reflector
   agent extracts at most one `{cue, lesson, evidence, confidence}` lesson per
   finished task; new tasks inject matching lessons (global + same root_domain)
   into planner/worker prompts. UI: `/function/lessons` page + dashboard card;
   API `GET /api/harness/lessons`. Deliberately NOT built: task-level
   propose–probe–compose re-execution (budget decision).

## Ported from ARTEX-HYDRA (see docs/forks/artex-hydra.md)

1. **[M] LLM failure classification** — DONE. `llmclass` + `llmrepair` ported
   (`llm/class`, `llm/repair`) with our additions (403-with-exhaustion override,
   subscription-window markers). `llm/pool` breaker, `agent.IsQuotaExhaustedMessage`
   (which `server/task_llm.go` uses) and `isHardFailure` all route through
   `llmclass.Classify` / `IsQuotaExhausted`; regression tests in pool + provider.
2. **[M] MCP server** — DONE (scoped to the intake use case). `mcpserver` +
   `mcpbuiltin` packages ported with their tests; our own thin wiring in
   `server/mcp.go`: `POST /mcp` (Streamable HTTP subset), Bearer
   `ARTEX_MCP_TOKEN` (endpoint not mounted at all without it), fixed narrow
   surface — `task_create` (normal creation path, never raw SQL), `task_status`,
   `task_list`. Their full L0-L3 capability registry is not ported.
3. **[L] Finding verifier agent** — DONE (scoped). Scanner applies severity policy
   (critical/high always, medium deterministic 50% sample, low never) to fresh
   findings; `finding_verifications` table + bound read-only "verifier"
   conversation agent (seeded, editable) with a frozen evidence snapshot; verdict
   (verified/refuted/inconclusive) mirrors onto `findings.validation`; reaper
   seals rows interrupted by restarts; history API
   `GET /api/exploration/findings/{id}/verifications`. Complements the
   report_finding verification gate (pre-filing) with a post-filing independent
   check. UI panel not built — API only for now.
4. **[M] Credential encryption at rest** — DONE (opt-in). `db/credential_crypto.go`
   (AES-256-GCM, `enc:v1:` prefix, lazy upgrade) + hooks: profile key encrypt on
   save / decrypt in the `scanProfile` funnel, web-search settings keys. Enabled
   by `ARTEX_CRED_ENCRYPT=1`; key file `credential.key` (0600) next to `jwt.key`;
   default off keeps existing deployments plaintext. Defense-in-depth on top of
   ADR-0001, not a replacement.
5. **[S] Selective skills import** — DONE. 8 packs imported from the strix skill
   set (idor, ssrf, authentication-jwt, oauth, graphql, xss, sql-injection,
   path-traversal-lfi-rfi), Apache-2.0, verbatim at the pinned upstream commit;
   `skills/NOTICE.md` records provenance. Bulk import of the 75-pack tree stays
   rejected.
6. **[L] SQLite backend option** — DONE (experimental core). `ARTEX_PG_DSN=file:/path/
   artex.db?...` opens SQLite via modernc.org/sqlite (no CGO): translated schema
   (`schema_sqlite.sql`, adopted from upstream and extended with our tables), a
   rewrite hook (casts / FOR UPDATE / ILIKE / advisory locks) funnelled through
   shadowed Query/Exec methods, custom SQL functions (`now`, `regexp`, GREATEST
   and friends in `sqlite_compat.go`), Go-assembled finding snapshots (no jsonb
   emulation), dialect-conditional json paths in the findings filter. Validated
   by `TestSQLiteCore` (settings, profiles, tasks, findings, activity,
   verification queue, credential crypto). NOT validated: archive/restore,
   notification delivery, report versions, traffic search — those stay PG-only
   until someone exercises them on SQLite. PG remains the default backend.
7. **[L] Desktop-friendly mode** — phase 1: SQLite option (item 6) + single-binary
   image with the UI embedded + compose path for local runs. Phase 2 (a desktop
   shell/tray) waits for real demand; no Wails port until then.

Explicitly rejected: the zh-only locale model, the coordinator replacing
planner/workers (study only), the editorial theme, wholesale skills import,
SQLite-for-the-droplet deployments (PG stays the server default).

## Demo mode (eye)

Threat model: video recording and live presales screen shares - only what is rendered on
the screen must be protected. Downloads, exports and devtools are out of scope (operator
decision 2026-11-02). Full analysis: `docs/demo-mode-arch-review.md` (in `docs/`).

1. **[M] Mask at the client transport, not per surface** — DONE. `http<T>()` in
   `web/src/lib/api.ts` walks every JSON response: operator regex rules run over every
   string, a field-name denylist (password/token/api_key/cookie/email/invite/…) masks
   structured secrets fully, and reveal-managed keys (name/title/api_key_hint) are
   exempt so `<Sensitive>` click-to-reveal keeps working. Covered by
   `web/src/lib/demo.test.ts`. Per-surface wiring stays only where reveal is needed.
2. **[S] Publish the field-name denylist from Go.** One source of truth for "what is
   sensitive", served through `/api/settings/demo` and consumed by the client transport.
   Ref: demo-mode-arch-review.md finding 4.
3. **[S] Fixture-secret render test.** Assert that a demo-mode render of fixture pages
   contains none of the fixture secrets (assert on text content, not pixels).
4. **[S] Starter rule library.** Offer Datadog-style starter rules (email, IPv4, hostname,
   bearer token) as clickable chips in the Demo mode settings card instead of free typing
   only. Ref: demo-mode-arch-review.md finding 5.

## Operations

- **[S] Diskguard** — DONE (2026-11-06). Disk-overflow protection: age-based auto-rotation of
  traffic blobs (>7d) and terminal-task artifacts (>14d, audited into task_provenance_cleanups),
  emergency pass at >=85% fs usage with a 24h age floor, `diskguard_enabled` setting,
  `GET /api/diskguard/status`; docker json-file logs capped in docker-compose.yml (50m x 3).
  Born from the multi-GB log/artifact incidents.

## Other known gaps

- Server-side credential redaction in activity/exploration payloads was considered and
  REJECTED for now (operator decision 2026-11-03). See
  `docs/adr/0001-credentials-unredacted-in-activity.md`. MUST be revisited before any
  multi-user deployment or export feature.
- (none other; the traffic capture item was fixed on 2026-11-02: the mechanism was already wired,
only the `traffic_capture` and `agent_traffic_binding` switches were off; enabling them via
PUT /api/settings started recording, verified end-to-end with a proxied request.)
