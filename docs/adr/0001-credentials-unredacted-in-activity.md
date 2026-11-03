# ADR-0001: Live credentials stay unredacted in activity and exploration payloads

Status: accepted (revisit later)
Date: 2026-11-03

## Context

Subagents write live credentials into `activity.summary/detail/metadata` and
`exploration_nodes.payload`: MAS access tokens (`mct_...`), invitations (`mas_...`),
cookie values. Any ARTEX user can read them. This was raised as defect 1 of the
customer-portal.example pentest runs (evidence: the operator's engagement notes).

## Decision

We do NOT redact credentials at the server before writing activity/exploration
payloads. Values are stored as-is.

## Rationale

- ARTEX is a single-operator white-hat harness. The audience of the database is the
  operator; there is no multi-tenant read exposure to defend against.
- Raw payloads are evidence: prefix-preserving redaction would still need a retained
  value somewhere for retest and reproduction, so the secret effectively survives
  either way. Redaction would give a false sense of hygiene without removing the
  underlying exposure.
- Tokens used in runs are disposable by procedure: after a run they are revoked
  (device deletion, invitation revocation, account deactivation), which makes stored
  copies inert. This operational control - not server-side redaction - is the actual
  protection.
- Server-side pattern matching (`mct_...`, cookie values) is brittle and would have
  to keep pace with new token formats of every target; the per-surface masking lesson
  from demo mode applies (see demo-mode-arch-review.md).

## Consequences

- Operator procedure, not code, keeps stored credentials harmless: revoke test
  devices/invitations/accounts after each run, as documented in the engagement notes.
- A future multi-user deployment or export feature MUST revisit this decision before
  shipping; see docs/backlog.md.
