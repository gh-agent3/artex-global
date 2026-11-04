#!/usr/bin/env bash
# Fork review metrics: prints the raw numbers used to fill docs/forks/<slug>.md.
#
#   git remote add <slug> <repo-url> && git fetch <slug>
#   ./scripts/fork-review.sh <slug>
#
# Note: after a history rewrite (filter-repo) our hashes no longer share ancestors
# with upstream-forked repos, so relationship is measured content-wise (diff vs dev),
# not via merge-base.

set -euo pipefail

SLUG=${1:-}
if [ -z "$SLUG" ] || ! git rev-parse --verify --quiet "$SLUG/main" >/dev/null; then
  echo "usage: $0 <remote-slug>   (a git remote whose default branch is <slug>/main)" >&2
  exit 2
fi

REF="$SLUG/main"
echo "== repo: $(git remote get-url "$SLUG")"
echo "== head: $(git log --oneline -1 "$REF")"
echo "== head date: $(git log -1 --format=%cI "$REF")"
echo "== total commits on branch: $(git rev-list --count "$REF")"
echo "== tags: $(git ls-remote --tags "$SLUG" 2>/dev/null | grep -v '\^' | wc -l) (last: $(git ls-remote --tags "$SLUG" 2>/dev/null | grep -v '\^' | tail -1 | sed 's|.*/||'))"
echo "== authors (top 5):"
git log "$REF" --format='%an' | sort | uniq -c | sort -rn | head -5 | sed 's/^/   /'
echo "== diff vs our dev ($(git log --oneline -1 dev | cut -d' ' -f1)):"
git diff --stat dev "$REF" | tail -1 | sed 's/^/   /'
echo "== changed top-level dirs:"
git diff --name-only dev "$REF" | cut -d/ -f1 | sort | uniq -c | sort -rn | head -10 | sed 's/^/   /'
echo "== commit type histogram:"
git log "$REF" --format='%s' | grep -oiE '^[a-z]+\(([^)]+)\)' | sort | uniq -c | sort -rn | head -8 | sed 's/^/   /'
