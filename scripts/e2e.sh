#!/usr/bin/env bash
# End-to-end run of the real server against a fresh flake repository and a bare
# git remote standing in for Forgejo: login, starter flake, lock, check, the
# commit gate, commit, push, edit, history, a push from elsewhere checked
# through the webhook, a trial on one machine, and the rollback of a failed
# deployment.
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
# The example machine's system derivation follows its default.nix.
case "$*" in
  *"eval --json"*) ;;
  *) echo "nix $*" ;;
esac
case "$*" in
  *"nixmgr:host"*) drv=$(sha256sum < "${NIXMGR_FLAKE#path:}/hosts/example/default.nix" | cut -c1-32)
    echo '{"name":"example","hostName":"example","system":"x86_64-linux","release":"26.05","stateVersion":"26.05","toplevel":"/nix/store/'"$drv"'-nixos-system-example-26.05.drv","boot":"","systemPath":"/nix/store/'"$drv"'-nixos-system-example-26.05","units":{"example.service":{"hash":"'"$drv"'","reload":{},"flags":{}}},"etc":{"example.conf":"'"$drv"'"},"managedFile":"hosts/example/managed.json","packages":[{"name":"git","version":"2.51.0","files":["hosts/example/managed.json"]}],"features":[],"users":[],"ports":{"tcp":[22],"udp":[],"firewall":true},"fileSystems":[],"files":["flake.nix"],"checks":{"comin":true,"cominBranches":[{"url":"https://forgejo.example.lan/pablo/nixos-config.git","main":"main","testing":"testing-example"}]}}' ;;
  *"eval --json"*) echo '{"nixos":["example"],"home":[],"darwin":[]}' ;;
  *"flake lock"*) d="${*##*path:}"; printf '{ "nodes": { "root": {} }, "root": "root", "version": 7 }\n' > "$d/flake.lock" ;;
  *"--dry-run"*) printf 'these 3 derivations will be built:\nthese 9 paths will be fetched (1.50 MiB download, 6.00 MiB unpacked):\n' >&2 ;;
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
if timeout 10 env NIXMGR_EVAL_JOBS=0 "$work/nixos-manager" > "$work/server.log" 2>&1 ||
  ! grep -q NIXMGR_EVAL_JOBS "$work/server.log"; then
  fail "NIXMGR_EVAL_JOBS=0 was accepted"
fi
: > "$work/server.log"
echo "ok   a bad NIXMGR_EVAL_JOBS stops the start"
export NIXMGR_EVAL_JOBS=2 # machines evaluated at once
start() {
  "$work/nixos-manager" >> "$work/server.log" 2>&1 &
  pid=$!
  for _ in $(seq 100); do curl -s -o /dev/null "$base/" && break; sleep 0.1; done
}
start

jar=$work/cookies
api() { local m=$1 p=$2; shift 2; curl -s -b "$jar" -c "$jar" -X "$m" -H 'Content-Type: application/json' "$base$p" "$@"; }
code() { local m=$1 p=$2; shift 2; api "$m" "$p" -o /dev/null -w '%{http_code}' "$@"; }
# run_job <json> waits for the job and prints its final state; built prints the
# machines the last one built.
run_job() {
  local id
  id=$(api POST /api/jobs -d "$1" | json '["id"]')
  echo "$id" > "$work/last-job"
  api GET "/api/jobs/$id/events" > "$work/events-$id"
  api GET "/api/jobs/$id" | json '["state"]'
}
built() { api GET "/api/jobs/$(cat "$work/last-job")" | json '["summary"]["built"]'; }
# ready <path> polls an endpoint that answers 202 while Nix works and prints the 200 body.
ready() {
  for _ in $(seq 600); do
    if [ "$(code GET "$1")" = 200 ]; then api GET "$1"; return; fi
    sleep 0.5
  done
  fail "$1 never became ready"
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
[ "$(built)" = "['example']" ] || fail "nothing pushed yet, so the check must build every machine: $(built)"
[ "$(api GET /api/status | json '["staged_check"]["built"]')" = "['example']" ] || fail "status does not say what the check built"
if [ "${E2E_STUB_NIX:-}" != 1 ]; then
  root=$(readlink "$NIXMGR_DATA/gcroots/example") || fail "no GC root for the example machine's system"
  case "$root" in /nix/store/*-nixos-system-example-*) ;; *) fail "the GC root points at $root" ;; esac
  [ -e "$root/activate" ] || fail "the built system $root has no activate script"
fi
echo "ok   check passed: flake check, and the example machine built"

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
[ "$(built)" = "['example']" ] || fail "the time zone changes the example machine, but the check built $(built)"
sha2=$(api POST /api/commit -d '{"message":"feat(example): set the time zone"}' | json '["sha"]')
# What the push dialog shows: the commit to push against what is pushed.
pushdiff=$(ready '/api/model/diff?against=pushed')
[ "$(echo "$pushdiff" | json '["hosts"][0]["status"]')" = changed ] || fail "the push changes the example machine: $pushdiff"
if [ "${E2E_STUB_NIX:-}" = 1 ]; then
  [ "$(echo "$pushdiff" | json '["hosts"][0]["units"]')" = "[{'name': 'example.service', 'action': 'restart'}]" ] || fail "units in the push diff: $pushdiff"
else # both systems were built by the two checks
  echo "$pushdiff" | json '["hosts"][0]["etc"]' | grep -q /etc/localtime || fail "the time zone's /etc/localtime is not in the push diff: $pushdiff"
  [ "$(echo "$pushdiff" | json '["hosts"][0]["closure"]["sizeTo"] > 0')" = True ] || fail "no closure in the push diff: $pushdiff"
fi
api POST /api/push -d '{}' > /dev/null
[ "$(git -C "$work/remote.git" rev-parse main)" = "$sha2" ] || fail "second push"
echo "ok   edit, diff, check, commit, what the push changes, push"

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

# finding <id> prints Health's finding with that id, or nothing.
finding() { ready /api/insights | python3 -c "import sys, json; print(next((json.dumps(i) for i in json.load(sys.stdin)['insights'] if i['id'] == '$1'), ''))"; }
forced=$(git -C "$work/remote.git" rev-parse main)
[ "$(finding unchecked-commit | json '["fix"]["check"]')" = "$forced" ] || fail "Health does not name the unchecked commit on main: $(finding unchecked-commit)"
echo "ok   Health names the unchecked commit on main"

# A commit pushed from another clone, announced by the signed webhook, is checked.
git clone -q "$work/remote.git" "$work/laptop"
echo '{ }' > "$work/laptop/modules/laptop.nix"
git -C "$work/laptop" add -A
git -C "$work/laptop" -c user.name=Laptop -c user.email=laptop@example.invalid commit -q -m "laptop: a new module"
git -C "$work/laptop" push -q
outside=$(git -C "$work/remote.git" rev-parse main)
secret=$(api GET /api/forgejo | json '["hook_secret"]')
body="{\"ref\":\"refs/heads/main\",\"before\":\"$forced\",\"after\":\"$outside\"}"
sig=$(python3 -c 'import hashlib, hmac, sys; print(hmac.new(sys.argv[1].encode(), sys.argv[2].encode(), hashlib.sha256).hexdigest())' "$secret" "$body")
[ "$(curl -s -o /dev/null -w '%{http_code}' -X POST -H 'Content-Type: application/json' -H 'X-Forgejo-Event: push' \
  -H "X-Forgejo-Signature: $sig" -d "$body" "$base/api/hooks/forgejo")" = 202 ] || fail "the push webhook"
state=
for _ in $(seq 1200); do
  state=$(api GET /api/jobs | python3 -c "import sys, json; print(next((j['state'] for j in json.load(sys.stdin)['jobs'] if j.get('commit') == '$outside'), ''))")
  case "$state" in passed | failed) break ;; esac
  sleep 0.5
done
[ "$state" = passed ] || fail "the check of the pushed commit: ${state:-never queued}"
[ "$(git -C "$repo_dir" rev-parse HEAD)" = "$outside" ] || fail "the console did not fast-forward to the pushed commit"
[ -z "$(finding unchecked-commit)" ] || fail "Health still names a commit: $(finding unchecked-commit)"
echo "ok   a commit pushed elsewhere is checked through the webhook"

# Try a change on the example machine alone: a commit on its testing branch, on
# top of main, pushed; the console then works on that branch.
hash=$(api GET "/api/file?path=$file" | json '["hash"]')
new=$(api GET "/api/file?path=$file" | python3 -c 'import sys, json; print(json.dumps(json.load(sys.stdin)["content"].replace("system.stateVersion", "time.timeZone = \"Europe/Paris\";\n  system.stateVersion")))')
api PUT /api/file -d "{\"path\":\"$file\",\"content\":$new,\"base_hash\":\"$hash\"}" > /dev/null
api POST /api/stage -d '{"paths":[]}' > /dev/null
[ "$(run_job '{"kind":"check"}')" = passed ] || fail "check before the trial: $(cat "$work"/events-*)"
ready '/api/model?at=working' > /dev/null
main=$(git -C "$work/remote.git" rev-parse main)
trial=$(api POST /api/try -d '{"host":"example","message":"try: Paris time"}')
tsha=$(echo "$trial" | json '["commit"]') || fail "try on example: $trial"
[ "$(git -C "$work/remote.git" rev-parse testing-example)" = "$tsha" ] || fail "testing-example is not the trial"
[ "$(git -C "$work/remote.git" rev-parse "$tsha^")" = "$main" ] || fail "the trial is not on top of main"
[ "$(git -C "$work/remote.git" rev-parse main)" = "$main" ] || fail "the trial moved main"
[ "$(api GET /api/status | json '["branch"]')" = testing-example ] || fail "the console is not on the testing branch"
dep=$(api GET /api/deployments | python3 -c "import sys, json; m = next(m for m in json.load(sys.stdin)['machines'] if m['host'] == 'example'); print(m['main'], m['testing'], m['branch'])")
[ "$dep" = "main testing-example testing-example" ] || fail "deployments: example follows $dep"
echo "ok   tried on example: testing-example on top of main, deployed there alone"

ended=$(api POST /api/try/end -d '{"host":"example"}' | json '["message"]')
[ "$(git -C "$work/remote.git" rev-parse testing-example)" = "$main" ] || fail "end trial: $ended"
[ "$(code POST /api/switch -d '{"name":"main"}')" = 204 ] || fail "back to main"
api POST /api/merge -d '{"branch":"testing-example"}' > /dev/null
[ "$(api GET /api/status | json '["staged_checked"]')" = True ] || fail "the promoted trial is not the tree that passed"
api POST /api/commit -d '{"message":"feat(example): Paris time"}' > /dev/null
[ "$(code POST /api/push -d '{}')" = 200 ] || fail "push the promoted trial without a new check"
promoted=$(git -C "$work/remote.git" rev-parse main)
echo "ok   trial ended, then promoted to main by the merge and pushed"

# example reports that it deployed main, then failed to build the promoted commit:
# the overview names that commit, and its revert goes through check, commit and push.
token=$(api GET '/api/deployments/token?host=example' | json '["token"]')
report() { curl -s -o /dev/null -w '%{http_code}' -X POST -H "Authorization: Bearer $token" -H 'Content-Type: application/json' -d "$1" "$base/api/deployments/report"; }
[ "$(report "{\"host\":\"example\",\"commit\":\"$main\",\"status\":\"done\"}")" = 204 ] || fail "deployment report"
[ "$(report "{\"host\":\"example\",\"commit\":\"$promoted\",\"phase\":\"build\",\"status\":\"failed\",\"error\":\"boom\"}")" = 204 ] || fail "failure report"
culprits=$(api GET /api/deployments | python3 -c "import sys, json; print([c['sha'] for m in json.load(sys.stdin)['machines'] if m['host'] == 'example' for c in m.get('culprits', [])])")
[ "$culprits" = "['$promoted']" ] || fail "culprits: $culprits"
[ "$(code POST /api/revert -d "{\"rev\":\"$promoted\",\"since\":\"$main\"}")" = 204 ] || fail "revert since the last deployment"
[ "$(run_job '{"kind":"check"}')" = passed ] || fail "check of the revert"
api POST /api/commit -d '{"message":"Revert the commit that broke example"}' > /dev/null
[ "$(code POST /api/push -d '{}')" = 200 ] || fail "push the revert"
[ "$(git -C "$work/remote.git" rev-parse 'main^{tree}')" = "$(git -C "$work/remote.git" rev-parse "$main^{tree}")" ] || fail "the revert does not bring back what example ran"
echo "ok   a failed deployment rolled back: revert, check, commit, push"

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
