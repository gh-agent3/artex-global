#!/usr/bin/env bash
# Build the ARTEX image from source.
#
#   ./build.sh /path/to/artex-checkout
#
# The checkout must be the ARTEX source tree (fork or upstream clone). The image
# is tagged artex-local:<commit> and labelled with the full commit.
set -euo pipefail

SRC=${1:-${ARTEX_SRC:-}}
if [ -z "$SRC" ] || [ ! -f "$SRC/go.mod" ]; then
  echo "usage: $0 /path/to/artex-checkout" >&2
  exit 2
fi

SHA=$(git -C "$SRC" rev-parse HEAD)
SHORT=$(git -C "$SRC" rev-parse --short HEAD)
VERSION=$(git -C "$SRC" describe --tags --always --dirty 2>/dev/null || echo "$SHORT")

docker build \
  -f "$(cd "$(dirname "$0")" && pwd)/Dockerfile.artex" \
  --build-arg ARTEX_GIT_SHA="$SHA" \
  --build-arg ARTEX_VERSION="${VERSION#v}" \
  -t "artex-local:$SHORT" \
  "$SRC"

# Tag with the binary hash as well: the plain commit tag is rebuilt in place,
# the hash tag names one exact binary and can be pinned in compose.yml.
BIN_HASH=$(docker run --rm --entrypoint sha256sum "artex-local:$SHORT" /app/artex | cut -d' ' -f1)
docker tag "artex-local:$SHORT" "artex-local:$SHORT-${BIN_HASH:0:8}"

echo "built artex-local:$SHORT-${BIN_HASH:0:8} (commit $SHA, binary sha256 $BIN_HASH)"
