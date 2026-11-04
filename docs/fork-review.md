# Fork review process

We track third-party ARTEX forks and adjacent repositories on a regular basis:
the operator adds a repo, we review what it changes relative to our platform and
record the assessment. One file per repo under `docs/forks/`.

## Adding a repo

1. Save it as a git remote (this is the durable copy; keep the remote):

   ```sh
   git remote add <slug> <repo-url>
   git fetch <slug>
   ```

2. Collect the metrics (prints the raw numbers, no writes):

   ```sh
   ./scripts/fork-review.sh <slug>
   ```

3. Write `docs/forks/<slug>.md` from the notes below and commit to `dev`.

## What goes into docs/forks/<slug>.md

- Metadata: url, slug, date first reviewed, branch reviewed, head commit, tags.
- Relationship: where it forked from and how far it has diverged (commit count,
  diff stat vs our `dev`, changed top-level dirs). Note: our history was rewritten
  (filter-repo), so merge-base against upstream-forked repos does not exist —
  always measure content-wise, diff against `dev`.
- Direction: what the fork is doing (language target, feature themes from the
  commit histogram, new packages).
- Assessment: what is worth taking (cherry-pick candidates), what conflicts with
  our direction (e.g. a different UI language), maintenance signal (commit
  velocity, CI health).
- Decision: `watch` (record only), `cherry-pick` (specific commits listed), or
  `ignore` (dead or hostile to our direction). Re-review on request or on new
  tags from the upstream fork.

## Current repos

- `artex-hydra` — see [forks/artex-hydra.md](forks/artex-hydra.md)
