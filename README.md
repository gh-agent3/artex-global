<div align="center">

# ARTEX — Global Edition

**An autonomous penetration-testing console driven by LLM multi-agents** (Go backend + Next.js frontend)

[![license: AGPL-3.0](https://img.shields.io/badge/license-AGPL--3.0-blue.svg)](LICENSE)

</div>

---

> ## Security & misuse warning — read this first
>
> This repository is published for use only within authorized environments, and only
> to build defensive and detection capabilities.

## What this fork is

A source-built, English-first deployment of the ARTEX platform:

- The Docker image builds the frontend and the Go binary from source — no prebuilt
  binary is copied in. The image is tied to the exact commit via build args and labels.
- UI and agent-generated output are English by default; Russian is available in
  Settings (System → Settings → Display language). Other locales (ko/zh/es/ja) ship
  with the upstream translation catalogs.
- Demo mode: one click masks sensitive fields for video recording and presales screen
  shares (see `docs/demo-mode.md`).
- Traffic capture: an MITM recording proxy captures worker HTTP/S traffic for evidence
  binding to findings.
- Finding quality gates: `report_finding` requires an explicit verification (source
  reference or clean-session reproduction) before a finding is filed.
- The version in the UI header links to the exact commit on GitHub; hover shows the
  commit subject and date.

## Build the image

Requires Docker with buildx. From the repository root:

```sh
./build-image.sh .
```

This produces `artex-local:<short-sha>` plus an immutable binary-hash tag
`artex-local:<short-sha>-<hash>` suitable for pinning in a compose file. Build-time
git metadata (commit, subject, date, repo URL) is embedded into the binary and served
via `/api/health`.

## Run

ARTEX needs PostgreSQL. A minimal working stack ships as `docker-compose.yml`
(PostgreSQL pinned by digest, artex pinned by the binary-hash tag):

```sh
./build-image.sh .
ARTEX_IMAGE=artex-local:<short-sha>-<hash> docker compose up -d
```

Then open `http://127.0.0.1:8787`. Health check: `GET /api/health`.

## Documentation

- `docs/fork-review.md` — the standing process for reviewing third-party forks
  (repos under review are saved as git remotes, one review file per repo in
  `docs/forks/`)
- `docs/demo-mode.md` — demo mode (screen-share masking) design and decisions
- `docs/backlog.md` — known gaps and pending improvements
- `docs/adr/` — architecture decision records

## Branches

- `main` — release branch; each release is tagged `v<version>` (e.g. `v0.3.16-global.1`)
- `dev` — active development

## License

AGPL-3.0, inherited from upstream (Autumn-27/ARTEX).
