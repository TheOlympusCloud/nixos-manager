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
# Evaluations answer with JSON only, like nix eval --json; other commands echo.
case "$*" in
  *"eval --json"*) ;;
  *) echo "nix $*" ;;
esac
case "$*" in
  *"nixmgr:host"*) echo '{"name":"example","hostName":"example","system":"x86_64-linux","release":"26.05","stateVersion":"26.05","managedFile":"hosts/example/managed.json","packages":[{"name":"git","version":"2.51.0","files":["hosts/example/managed.json"]}],"features":[],"users":[],"ports":{"tcp":[22],"udp":[],"firewall":true},"fileSystems":[],"files":["flake.nix"]}' ;;
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
start() {
  "$work/nixos-manager" >> "$work/server.log" 2>&1 &
  pid=$!
  for _ in $(seq 100); do curl -s -o /dev/null "$base/" && break; sleep 0.1; done
}
start

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

# Lock runs on an export: an untracked file, an ignored one and .git stay out of the store.
repo_dir=$work/data/repo
echo e2e > "$repo_dir/e2e-untracked"
echo e2e > "$repo_dir/e2e-ignored"
echo e2e-ignored >> "$repo_dir/.git/info/exclude"
[ "$(run_job '{"kind":"lock"}')" = passed ] || fail "nix flake lock: $(cat "$work"/events-*)"
api GET '/api/file?path=flake.lock' | json '["hash"]' > /dev/null || fail "no flake.lock after lock"
if [ "${E2E_STUB_NIX:-}" != 1 ]; then
  for p in /nix/store/*-source; do
    if [ -e "$p/e2e-untracked" ] || [ -e "$p/e2e-ignored" ] || grep -qsF "$work/remote.git" "$p/.git/config"; then
      fail "lock copied the working copy into $p"
    fi
  done
fi
rm "$repo_dir/e2e-untracked" "$repo_dir/e2e-ignored"
echo "ok   flake locked, from an export of the working copy"

[ "$(code POST /api/stage -d '{"paths":[]}')" = 204 ] || fail "stage"
attr=$(api GET /api/targets | json '["targets"][0]["attr"]') || fail "targets"
[ "$attr" = "nixosConfigurations.example.config.system.build.toplevel" ] || fail "unexpected target $attr"
echo "ok   targets: $attr"

[ "$(code POST /api/commit -d '{"message":"feat: initial machines"}')" = 412 ] || fail "commit allowed before a check"
state=$(run_job "{\"kind\":\"check\",\"targets\":[\"$attr\"]}")
[ "$state" = passed ] || fail "check $state: $(cat "$work"/events-*)"
[ "$(api GET /api/status | json '["staged_checked"]')" = True ] || fail "staged changes not marked checked"
echo "ok   check passed (flake check + dry build)"

kill "$pid"; wait "$pid" 2>/dev/null || true
start
[ "$(code GET /api/status)" = 200 ] || fail "the restart logged the session out"
[ "$(api GET /api/status | json '["staged_checked"]')" = True ] || fail "the restart forgot the passed check"
echo "ok   a restart keeps the session and the passed check"

sha=$(api POST /api/commit -d '{"message":"feat: initial machines"}' | json '["sha"]') || fail "commit"
api POST /api/push -d '{}' > /dev/null
[ "$(git -C "$work/remote.git" rev-parse main)" = "$sha" ] || fail "remote does not have the commit"
[ "$(git -C "$work/remote.git" log -1 --format='%an <%ae>|%cn <%ce>' main)" = "E2E Person <e2e@example.invalid>|E2E Person <e2e@example.invalid>" ] || fail "wrong commit identity"
git -C "$work/remote.git" log -1 --format='%(trailers:key=Via,valueonly)' main | grep -Eq '^web session [0-9a-f]{8}$' || fail "commit lacks the Via trailer"
echo "ok   committed and pushed as the configured person, via the web session"

file=hosts/example/default.nix
hash=$(api GET "/api/file?path=$file" | json '["hash"]')
new=$(api GET "/api/file?path=$file" | python3 -c 'import sys, json; print(json.dumps(json.load(sys.stdin)["content"].replace("system.stateVersion", "time.timeZone = \"Europe/Madrid\";\n  system.stateVersion")))')
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
[ "$(code POST /api/restore -d "{\"path\":\"$file\",\"rev\":\"$sha\",\"base_hash\":\"stale\"}")" = 409 ] || fail "restore over an unseen version not refused"
hash=$(api GET "/api/file?path=$file" | json '["hash"]')
[ "$(code POST /api/restore -d "{\"path\":\"$file\",\"rev\":\"$sha\",\"base_hash\":\"$hash\"}")" = 204 ] || fail "restore"
api GET "/api/file?path=$file" | json '["content"]' | grep -q timeZone && fail "restore did not bring back the old version"
echo "ok   history and restore"

api PUT /api/file -d '{"path":"modules/new.nix","content":"{ }\n","base_hash":""}' > /dev/null
api POST /api/stage -d '{"paths":[]}' > /dev/null
api POST /api/commit -d '{"message":"chore: unchecked change","force":true}' > /dev/null
[ "$(code POST /api/push -d '{}')" = 412 ] || fail "unchecked commit pushed without force"
[ "$(code POST /api/push -d '{"force":true}')" = 200 ] || fail "forced push"
echo "ok   push needs a checked commit unless forced"

# ready <path> polls an endpoint that answers 202 while Nix works and prints the 200 body.
ready() {
  for _ in $(seq 600); do
    if [ "$(code GET "$1")" = 200 ]; then api GET "$1"; return; fi
    sleep 0.5
  done
  fail "$1 never became ready"
}
model=$(ready '/api/model?at=working')
[ "$(echo "$model" | json '["model"]["hosts"][0]["name"]')" = example ] || fail "fleet model: $model"
[ "$(echo "$model" | json '["model"].get("error", "")')" = "" ] || fail "the starter flake does not evaluate: $model"
[ "$(echo "$model" | json '["model"]["hosts"][0].get("error", "")')" = "" ] || fail "the example machine does not evaluate: $model"
if [ "${E2E_STUB_NIX:-}" != 1 ]; then # read from services.comin.remotes in modules/common.nix
  branches=$(echo "$model" | json '["model"]["hosts"][0]["checks"]["cominBranches"]')
  [ "$branches" = "[{'url': 'https://forgejo.example.lan/pablo/nixos-config.git', 'main': 'main', 'testing': 'testing-example'}]" ] ||
    fail "comin's branches: $branches"
fi
echo "ok   fleet model: the example machine evaluates"

[ "$(code POST /api/hosts/example/packages -d '{"add":["ripgrep"]}')" = 200 ] || fail "add a package by clicking"
api GET /api/hosts/example/managed | json '["packages"]' | grep -q ripgrep || fail "managed.json lacks the package"
api GET /api/status | json '["files"]' | grep -q 'hosts/example/managed.json' || fail "the edit is not a git change"
if [ "${E2E_STUB_NIX:-}" != 1 ]; then # the stub cannot evaluate the edit
  diff=$(ready /api/model/diff)
  echo "$diff" | json '["hosts"][0].get("packagesAdded", [])' | grep -q ripgrep || fail "the diff does not show the new package: $diff"
fi
echo "ok   package added by clicking shows up as a change"

echo "PASS"
