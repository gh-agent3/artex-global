# ARTEX.sh top-3 + FailSafe analysis (2026-11-18)

Source: https://www.artex.sh/forks/best (GitHub data pulled 2026-11-16). Our
line: ACP Edition @ 8cdef3bf+ (this review adds the self-update hardening).

## Top-3 by "most active" ranking

### 1. jaykwok/ARTEX (v1.0.5, 91 stars, push 2026-11-16)

- 20 commits over v0.3.15. Headline: **embedded SQLite storage** (a real
  no-Postgres implementation — their v1.0 line is built around it).
- Also removed the .gov/.edu target blocklist and the usage disclaimer
  (commit 7bd8dc2). That is a legal/scope change in the hostile direction —
  we explicitly do NOT adopt it; our blocklist stays.
- What is genuinely better than ours: SQLite is their DEFAULT and deeply
  integrated (ours is an experimental option, core paths only). Their
  v1.0.5 also ships Go 1.27.4 toolchain fixes.
- Take: nothing to port except the reminder that our SQLite coverage is the
  gap (already backlog item 6).

### 2. wpsec/ARTEX-Pro (v0.3.17, 9 stars, push 2026-11-16)

- 98 commits over v0.3.15, independently maintained ("ARTEX-Pro").
- Theme histogram: 43 fix(core) + 8 feat(core) + a dedicated
  **feat(security)/fix(security) line**: dependency patching, auth and
  workspace-path risk fixes, skill-upload authorization defaults, bypass
  context-capture fixes.
- What is better than ours: sheer maintenance cadence on core, and an
  explicit security-fix track. We cherry-pick nothing blindly (their fork
  diverges heavily and we cannot audit 98 commits in one pass), but their
  fix(security) commits are a checklist to mine: auth/workspace-path risks,
  skill-upload authorization defaults.
- Take: `watch`; mine the fix(security) commits one by one before our next
  release.

### 3. RuoJi6/ARTEX (v0.3.30, 2,098 stars, push 2026-11-14)

- 292 commits over v0.3.15 — the momentum leader.
- Notable: **exposes ARTEX as an MCP server for external agents** with a
  source allowlist (their PR #155); English i18n; Docker Hub images built by
  GitHub Actions; a public demo.
- What is better than ours: (a) their MCP-server has a source ALLOWLIST
  (our MCP ingress is Bearer-token only — no caller allowlist; worth
  adding); (b) release engineering (CI-built images, demo env); (c) fix
  volume on web/findings/intercept/demo surfaces.
- Take: `watch`; add an IP/source allowlist option to our MCP ingress
  (small, high value). Do not chase i18n/demo.

## failsafe-security/artex (774 stars, derivative #1) — "backdoors removed"

The incident: upstream shipped prebuilt Docker images under `tomicdd/artex`
that contained an **unauthenticated command-execution backdoor** (found
November 2026). FailSafe's fork exists as the source-built answer.

What they changed vs v0.3.15 (their README's own table + diff):

- Source build instead of the backdoored prebuilt image (we already build
  from source — unaffected by that backdoor).
- **Online self-update compiled OFF** (`selfupdate.Enabled=false`,
  `ErrDisabled`; updateCheck/updateApply/updateRollback refuse before any
  network; Stage/FetchLatest guard). ADOPTED — same implementation ported
  to our `internal/selfupdate` (`TestOnlineSelfUpdateCompiledOut`).
- `npx @playwright/mcp` launcher -> pinned local `playwright-mcp`.
  ADOPTED: image pins `@playwright/mcp@0.0.82`/`@playwright/cli@0.1.25`
  (0.0.82 is the newest mcp whose playwright dep matches our pinned
  Chromium 1.64), and the seeded browser MCP now launches
  `/usr/bin/playwright-mcp` with a one-time DB migration for existing
  installs.
- Base images pinned by digest (we already pin).
- no-new-privileges (we already have it).
- Compose binds 127.0.0.1 only and does not publish the recording proxy
  (our repo compose publishes only 8787; the droplet is reached through an
  ssh tunnel — equivalent posture; kept as is).
- api-recon dependencies preinstalled from a locked registry at image
  build time (we install at first use via npm with a committed lockfile).
  NOT adopted — recorded as backlog (image preinstall).
- Brotli/gzip static UI + cache validators (performance hardening). NOT
  adopted — cosmetic for our use.

Verdict on the source tree: v0.3.15 source itself is what FailSafe audited
and kept (with runtime capabilities intact); the backdoor lived in the
prebuilt image layer. Combined with our source build + now-disabled
self-update, the same class of attack does not have a delivery path here.
