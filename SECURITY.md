# Security

## Reporting a vulnerability

Do not open a public issue for a security vulnerability.

Report through **GitHub Security Advisories**: the **Security** tab ->
**Report a vulnerability** in `gh-agent3/artex-global`. That is the only
reporting channel.

Please include:

- a description of the vulnerability and its expected impact,
- the affected surface (API route, agent tool, MCP endpoint, ...),
- a minimal reproduction (request/command/steps),
- the version string from `GET /api/health` of the affected deployment.

## Security posture of this distribution

- Built from source only (`Dockerfile.artex`); base images pinned by digest.
- Online self-update is compiled out: the binary never checks or downloads a
  remote release (the runtime self-binary-swap channel is closed; we deploy by
  image). See `internal/selfupdate/github.go` (`Enabled = false`).
- This call mirrors failsafe-security/artex, published after the backdoored
  prebuilt `tomicdd/artex` image was found (November 2026): that incident was a
  prebuilt-image compromise, and the mitigations are source builds, pinned
  dependencies, and no runtime self-replacement.

## Scope notes

- Only the current release line receives security fixes; run the latest tag.
- Modified/internal builds are not covered by any upstream support.
