#!/usr/bin/env bash
# make deploy (K05): build the COMMITTED tree, restart menata-app.service, prove the running binary is the
# one just built, and smoke five routes. The recurring failure it ends is "the process running is not
# bin/server": a restart that did not happen, or a binary built from someone else's half-finished edits.
#
# It builds `git archive HEAD`, not the working tree. Other sessions share this checkout and their
# uncommitted Go would otherwise ship from here; and it never runs `make generate`, because only the UI
# session regenerates *_templ.go / app.css (CLAUDE.md, "Working with other agent sessions"). Consequence:
# what deploys is exactly what was pushed. Restart only from the session that just pushed.
set -euo pipefail

cd "$(dirname "$0")/.."
unit=menata-app
base=${DEPLOY_BASE_URL:-http://localhost:4000}

src=$(mktemp -d)
trap 'rm -rf "$src"' EXIT
git archive HEAD | tar -x -C "$src"
commit=$(git rev-parse --short HEAD)

echo "==> building $commit from the committed tree"
(cd "$src" && go build -o "$OLDPWD/bin/server.new" ./cmd/server)
mv bin/server.new bin/server   # atomic: the running process keeps its old inode until restarted
want=$(sha256sum bin/server | cut -d' ' -f1)

echo "==> restarting $unit"
systemctl restart "$unit"

# The binary a process runs is /proc/<pid>/exe, which survives the file being replaced.
for _ in $(seq 1 30); do
  [ "$(systemctl is-active "$unit")" = active ] && break
  sleep 1
done
pid=$(systemctl show -p MainPID --value "$unit")
if [ -z "$pid" ] || [ "$pid" = 0 ]; then echo "FAIL: $unit has no main process" >&2; exit 1; fi
got=$(sha256sum "/proc/$pid/exe" | cut -d' ' -f1)
if [ "$got" != "$want" ]; then
  echo "FAIL: pid $pid runs a different binary than bin/server ($got != $want)" >&2
  exit 1
fi
echo "==> pid $pid runs bin/server ($commit)"

# Wait for the listener, then smoke. /home is authenticated: a 303 to /login is the healthy answer.
for _ in $(seq 1 30); do
  curl -s -o /dev/null "$base/login" && break
  sleep 1
done
css=$(curl -s "$base/login" | grep -o '/css/app\.[0-9a-f]*\.css' | head -1)
fail=0
check() { # path expected-status
  code=$(curl -s -o /dev/null -w '%{http_code}' "$base$1")
  if [ "$code" = "$2" ]; then echo "  ok   $1 $code"; else echo "  FAIL $1 $code (want $2)" >&2; fail=1; fi
}
check /login 200
check /register 200
check /forgot-password 200
check "${css:-/css/app.css}" 200
check /home 303
[ "$fail" = 0 ] || { echo "FAIL: smoke failed -- see /var/log/menata-app/app.log" >&2; exit 1; }
echo "==> deployed $commit"
