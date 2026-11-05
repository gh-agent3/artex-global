# Backlog

Things decided but not yet implemented. Newest on top within a section.

## Ported from ARTEX-HYDRA (see docs/forks/artex-hydra.md)

Adopted plan (operator decision 2026-11-05). Order is priority.

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
6. **[L] SQLite backend option** — RE-SCOPED (measurement 2026-11-05). Upstream's
   SQLite support is a platform subsystem, not a flag: 16 files / ~180 KB of
   dialect and adapter code plus a separate 96 KB SQLite schema
   (dialect_arrays/companies/intercept/locking/mixed/softdelete, open_sqlite,
   json scan adapters, error mapping), all woven through a db layer that has
   since diverged from ours. Honest estimate: a multi-day port with per-query
   validation, not a session item. Until it lands, `db.Open` fails fast with a
   clear message on sqlite DSNs. PG stays the only backend.
7. **[L] Desktop-friendly mode** — phase 1 delivered as far as possible without
   item 6: the release image is a single binary with the UI embedded, and the
   local/desktop path is `./build-image.sh . && docker compose up` (README).
   A bundled no-Postgres single-user mode waits on item 6. No Wails shell until
   that proves out.

Explicitly rejected: the zh-only locale model, the coordinator replacing
planner/workers (study only), the editorial theme, wholesale skills import,
SQLite-for-the-droplet deployments (PG stays the server default).

## Demo mode (eye)

Threat model: video recording and live presales screen shares - only what is rendered on
the screen must be protected. Downloads, exports and devtools are out of scope (operator
decision 2026-11-02). Full analysis: `demo-mode-arch-review.md`; design: `docs/demo-mode.md`.

1. **[M] Mask at the client transport, not per surface.** Move masking into `http<T>()` in
   `web/src/lib/api.ts`: walk each JSON response, replace fields on a sensitive-name
   denylist and apply the operator regex rules to every string value. Delete the per-surface
   masking (exploration-graph payload, DemoMarkdown, report-tab rules) - the transport
   covers every current and future surface. Keep `<Sensitive>` only where click-to-reveal
   is needed (LLM key hints, task names); exempt those field names from transport masking.
   Ref: demo-mode-arch-review.md finding 2.
2. **[S] Publish the field-name denylist from Go.** One source of truth for "what is
   sensitive", served through `/api/settings/demo` and consumed by the client transport.
   Ref: demo-mode-arch-review.md finding 4.
3. **[S] Fixture-secret render test.** Assert that a demo-mode render of fixture pages
   contains none of the fixture secrets (assert on text content, not pixels).
4. **[S] Starter rule library.** Offer Datadog-style starter rules (email, IPv4, hostname,
   bearer token) as clickable chips in the Demo mode settings card instead of free typing
   only. Ref: demo-mode-arch-review.md finding 5.

## Other known gaps

- Server-side credential redaction in activity/exploration payloads was considered and
  REJECTED for now (operator decision 2026-11-03). See
  `docs/adr/0001-credentials-unredacted-in-activity.md`. MUST be revisited before any
  multi-user deployment or export feature.
- (none other; the traffic capture item was fixed on 2026-11-02: the mechanism was already wired,
only the `traffic_capture` and `agent_traffic_binding` switches were off; enabling them via
PUT /api/settings started recording, verified end-to-end with a proxied request.)
