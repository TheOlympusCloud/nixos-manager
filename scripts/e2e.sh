#!/usr/bin/env bash
# End-to-end run of the real server against a fresh flake repository and a bare
# git remote standing in for Forgejo: login, starter flake, lock, check, the
# commit gate, commit, push, edit, history.
#
# Uses the real nix and nixfmt from PATH (network needed for the flake inputs).
# E2E_STUB_NIX=1 replaces them with stubs, for machines without Nix.
# Needs: go, git, curl, python3.
set -euo pipefail

repo=$(cd "$(dirname "$0")/.." && pwd)
work=$(mktemp -d)
port=${E2E_PORT:-18088}
base="http://127.0.0.1:$port"
pid=""
cleanup() {
  if [ -n "$pid" ]; then kill "$pid" 2>/dev/null || true; wait "$pid" 2>/dev/null || true; fi
  chmod -R u+w "$work" 2>/dev/null || true
  rm -rf "$work"
}
trap cleanup EXIT

fail() { echo "FAIL: $*" >&2; echo "--- server log" >&2; cat "$work/server.log" >&2; exit 1; }
json() { python3 -c "import sys, json; print(json.load(sys.stdin)$1)"; }

# Keep the caller's git config and hooks out of the throwaway repositories.
export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1
unset GIT_CONFIG_COUNT GIT_CONFIG_KEY_0 GIT_CONFIG_VALUE_0

nix_bin=nix nixfmt_bin=nixfmt
if [ "${E2E_STUB_NIX:-}" = 1 ]; then
  mkdir -p "$work/bin"
  cat > "$work/bin/nix" <<'EOF'
#!/bin/sh
echo "nix $*"
case "$*" in
  *"eval --json"*) echo '{"nixos":["example"],"home":[],"darwin":[]}' ;;
  *"flake lock"*) d="${*##*path:}"; printf '{ "nodes": { "root": {} }, "root": "root", "version": 7 }\n' > "$d/flake.lock" ;;
esac
EOF
  printf '#!/bin/sh\ncat\n' > "$work/bin/nixfmt"
  chmod +x "$work/bin/nix" "$work/bin/nixfmt"
  nix_bin=$work/bin/nix nixfmt_bin=$work/bin/nixfmt
fi

go build -C "$repo" -o "$work/nixos-manager" ./cmd/nixos-manager
git init -q --bare -b main "$work/remote.git"

export NIXMGR_DATA=$work/data NIXMGR_LISTEN=127.0.0.1:$port NIXMGR_REMOTE=$work/remote.git \
  NIXMGR_AUTHOR_NAME="E2E Person" NIXMGR_AUTHOR_EMAIL="e2e@example.invalid" \
  NIXMGR_NIX=$nix_bin NIXMGR_NIXFMT=$nixfmt_bin
NIXMGR_PASSWORD_HASH=$(printf 'pw\n' | "$work/nixos-manager" hash-password)
export NIXMGR_PASSWORD_HASH
"$work/nixos-manager" > "$work/server.log" 2>&1 &
pid=$!
for _ in $(seq 100); do curl -s -o /dev/null "$base/" && break; sleep 0.1; done

jar=$work/cookies
api() { local m=$1 p=$2; shift 2; curl -s -b "$jar" -c "$jar" -X "$m" -H 'Content-Type: application/json' "$base$p" "$@"; }
code() { local m=$1 p=$2; shift 2; api "$m" "$p" -o /dev/null -w '%{http_code}' "$@"; }
# run_job <json> waits for the job and prints its final state.
run_job() {
  local id
  id=$(api POST /api/jobs -d "$1" | json '["id"]')
  api GET "/api/jobs/$id/events" > "$work/events-$id"
  api GET "/api/jobs/$id" | json '["state"]'
}

[ "$(curl -s -o /dev/null -w '%{http_code}' "$base/api/tree")" = 401 ] || fail "API reachable without login"
[ "$(code POST /api/login -d '{"password":"wrong"}')" = 401 ] || fail "wrong password accepted"
[ "$(code POST /api/login -d '{"password":"pw"}')" = 204 ] || fail "login"
echo "ok   login required"

api GET /api/tree | json '["entries"]' | grep -q "'flake.nix'" || fail "starter flake not written"
echo "ok   new repository got the starter flake"

[ "$(run_job '{"kind":"lock"}')" = passed ] || fail "nix flake lock: $(cat "$work"/events-*)"
api GET '/api/file?path=flake.lock' | json '["hash"]' > /dev/null || fail "no flake.lock after lock"
echo "ok   flake locked"

[ "$(code POST /api/stage -d '{"paths":[]}')" = 204 ] || fail "stage"
attr=$(api GET /api/targets | json '["targets"][0]["attr"]') || fail "targets"
[ "$attr" = "nixosConfigurations.example.config.system.build.toplevel" ] || fail "unexpected target $attr"
echo "ok   targets: $attr"

[ "$(code POST /api/commit -d '{"message":"feat: initial machines"}')" = 412 ] || fail "commit allowed before a check"
state=$(run_job "{\"kind\":\"check\",\"targets\":[\"$attr\"]}")
[ "$state" = passed ] || fail "check $state: $(cat "$work"/events-*)"
[ "$(api GET /api/status | json '["staged_checked"]')" = True ] || fail "staged changes not marked checked"
echo "ok   check passed (flake check + dry build)"

sha=$(api POST /api/commit -d '{"message":"feat: initial machines"}' | json '["sha"]') || fail "commit"
api POST /api/push -d '{}' > /dev/null
[ "$(git -C "$work/remote.git" rev-parse main)" = "$sha" ] || fail "remote does not have the commit"
[ "$(git -C "$work/remote.git" log -1 --format='%an <%ae>|%cn <%ce>' main)" = "E2E Person <e2e@example.invalid>|E2E Person <e2e@example.invalid>" ] || fail "wrong commit identity"
echo "ok   committed and pushed as the configured person"

file=hosts/example/default.nix
hash=$(api GET "/api/file?path=$file" | json '["hash"]')
new=$(api GET "/api/file?path=$file" | python3 -c 'import sys, json; print(json.dumps(json.load(sys.stdin)["content"].replace("services.openssh.enable = true;", "services.openssh.enable = true;\n  time.timeZone = \"Europe/Madrid\";")))')
[ "$(code PUT /api/file -d "{\"path\":\"$file\",\"content\":$new,\"base_hash\":\"$hash\"}")" = 200 ] || fail "save"
[ "$(code PUT /api/file -d "{\"path\":\"$file\",\"content\":\"{ }\",\"base_hash\":\"$hash\"}")" = 409 ] || fail "stale save not refused"
api GET "/api/diff?staged=false&path=$file" | json '["diff"]' | grep -q '+  time.timeZone' || fail "diff"
api POST /api/stage -d "{\"paths\":[\"$file\"]}" > /dev/null
[ "$(run_job "{\"kind\":\"check\",\"targets\":[\"$attr\"]}")" = passed ] || fail "second check"
sha2=$(api POST /api/commit -d '{"message":"feat(example): set the time zone"}' | json '["sha"]')
api POST /api/push -d '{}' > /dev/null
[ "$(git -C "$work/remote.git" rev-parse main)" = "$sha2" ] || fail "second push"
echo "ok   edit, diff, check, commit, push"

[ "$(api GET "/api/log?path=$file" | json '["commits"].__len__()')" = 2 ] || fail "file history"
[ "$(code POST /api/restore -d "{\"path\":\"$file\",\"rev\":\"$sha\"}")" = 204 ] || fail "restore"
api GET "/api/file?path=$file" | json '["content"]' | grep -q timeZone && fail "restore did not bring back the old version"
echo "ok   history and restore"

echo "PASS"
