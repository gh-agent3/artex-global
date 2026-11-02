# Backlog

Things decided but not yet implemented. Newest on top within a section.

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

(none - the traffic capture item was fixed on 2026-11-02: the mechanism was already wired,
only the `traffic_capture` and `agent_traffic_binding` switches were off; enabling them via
PUT /api/settings started recording, verified end-to-end with a proxied request.)
