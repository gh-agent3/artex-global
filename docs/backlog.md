# Backlog

Things decided but not yet implemented. Newest on top within a section.

## Ported from ARTEX-HYDRA (see docs/forks/artex-hydra.md)

Adopted plan (operator decision 2026-11-05). Order is priority.

1. **[M] LLM failure classification** — DONE. `llmclass` + `llmrepair` ported
   (`llm/class`, `llm/repair`) with our additions (403-with-exhaustion override,
   subscription-window markers). `llm/pool` breaker, `agent.IsQuotaExhaustedMessage`
   (which `server/task_llm.go` uses) and `isHardFailure` all route through
   `llmclass.Classify` / `IsQuotaExhausted`; regression tests in pool + provider.
2. **[M] MCP server** — port `mcpserver/` (builtin/guard/http/probe) so external
   orchestrators (paperclip) can hand ARTEX tasks over MCP.
3. **[L] Finding verifier agent** — independent read-only re-verification pipeline
   (high severity mandatory, medium/low sampled) driving a validation state on
   findings; complements the report_finding verification gate.
4. **[M] Credential encryption at rest** — optional (off by default), key from
   env/file (0600, rotatable); encrypts stored tokens/invites. Defense-in-depth on
   top of ADR-0001, not a replacement.
5. **[S] Selective skills import** — pick 5-10 vuln-class packs from the strix skill
   set (idor, ssrf, jwt, oauth, graphql...), translate to English, review each;
   do NOT bulk-import the 75-pack tree.
6. **[L] SQLite backend option** — dialect layer so a single-user/local deployment
   can run without PostgreSQL. Enabler for a desktop-friendly mode.
7. **[L] Desktop-friendly mode** — phase 1: SQLite option + single-binary local run;
   phase 2 (evaluate later): thin desktop shell/tray. No Wails port until phase 1
   proves out.

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
