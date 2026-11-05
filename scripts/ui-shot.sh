#!/usr/bin/env bash
# Take an authenticated screenshot of the ARTEX UI.
#
#   ./scripts/ui-shot.sh [path] [out.png] [--demo] [--clip]
#
#   path   UI route, default /dashboard/        (leading/trailing / optional)
#   out    output png, default /tmp/opencode/ui-shot.png
#   --demo pre-set the demo cookie so the shot shows the masked UI
#   --clip viewport only, no full-page capture
#
# The wrapper ensures the ssh tunnel to the droplet, mints a login JWT from
# jwt.key + auth.password_hash (no password needed), and runs playwright inside
# the artex image container (it ships node + chromium).
set -euo pipefail

DROPLET=${ARTEX_HOST:-vladb@188.166.81.194}
PORT=8787
DEMO=0
FULL=1
PATH_ARG=""
OUT_ARG=""

for a in "$@"; do
  case "$a" in
    --demo) DEMO=1 ;;
    --clip) FULL=0 ;;
    /*|./*) [ -z "$PATH_ARG" ] && PATH_ARG="$a" || OUT_ARG="$a" ;;
    *) [ -z "$OUT_ARG" ] && OUT_ARG="$a" ;;
  esac
done
ROUTE=${PATH_ARG:-/dashboard/}
case "$ROUTE" in /*) ;; *) ROUTE="/$ROUTE" ;; esac
case "$ROUTE" in */) ;; *) ROUTE="$ROUTE/" ;; esac
OUT=${OUT_ARG:-/tmp/opencode/ui-shot.png}

# 1. tunnel
if ! curl -sf -o /dev/null "http://127.0.0.1:$PORT/api/health" --max-time 4; then
  ssh -f -N -o BatchMode=yes -L 127.0.0.1:$PORT:127.0.0.1:$PORT "$DROPLET"
  sleep 2
fi

# 2. mint a login JWT (same construction the API accepts: HS256 over the
#    password hash, keyed by jwt.key content).
JWT=$(ssh -o BatchMode=yes "$DROPLET" 'sudo -n docker exec artex-iso-artex-1 cat /app/jwt.key' | python3 -c '
import hmac, hashlib, base64, json, sys, subprocess, time
key = sys.stdin.read().strip()
ph = subprocess.run(["ssh","-o","BatchMode=yes","'"$DROPLET"'",
  "sudo -n docker exec artex-iso-postgres-1 psql -U artex -d artex -tAc",
  "select value from settings where key='"'"'auth.password_hash'"'"'"], capture_output=True, text=True).stdout.strip()
secret = hmac.new(key.encode(), ph.encode(), hashlib.sha256).digest()
b64 = lambda b: base64.urlsafe_b64encode(b).rstrip(b"=").decode()
h, p = b64(json.dumps({"alg":"HS256","typ":"JWT"}).encode()), b64(json.dumps({"sub":"ARTEX","exp":int(time.time())+900,"iat":int(time.time())}).encode())
print(h+"."+p+"."+b64(hmac.new(secret,(h+"."+p).encode(),hashlib.sha256).digest()))
')
[ -n "$JWT" ] || { echo "failed to mint JWT" >&2; exit 1; }

# 3. run playwright inside the artex image (node + chromium ship with it)
IMG=$(docker images --format '{{.Repository}}:{{.Tag}}' | grep '^artex-local:' | grep -- '-' | head -1)
[ -n "$IMG" ] || { echo "artex-local image not found locally" >&2; exit 1; }
mkdir -p "$(dirname "$OUT")"
OUT_BASE=$(basename "$OUT")
docker run --rm --network host \
  -e JWT="$JWT" -e "URL=http://127.0.0.1:$PORT$ROUTE" \
  -e "OUT=/out/$OUT_BASE" -e "DEMO=$DEMO" -e "FULL=$FULL" \
  -v "$(cd "$(dirname "$OUT")" && pwd):/out" \
  -v "$(cd "$(dirname "$0")" && pwd):/scripts:ro" \
  --entrypoint node "$IMG" /scripts/ui-shot.cjs
echo "$OUT"
