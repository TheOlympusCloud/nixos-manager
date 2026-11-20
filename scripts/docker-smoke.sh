#!/usr/bin/env bash
# Builds the Docker image and checks that the server runs as the unprivileged
# nixos-manager user: a volume written by the earlier image (which ran as root) is
# handed over and keeps working, real Nix locks and evaluates the starter flake as
# that user, and a new volume works without root at all (docker run --user).
#
# Needs: docker, curl, python3, and network (the image build and the flake inputs).
# SMOKE_IMAGE=<tag> with SMOKE_NO_BUILD=1 tests an image built beforehand.
set -euo pipefail

repo=$(cd "$(dirname "$0")/.." && pwd)
image=${SMOKE_IMAGE:-nixos-manager:smoke}
port=${SMOKE_PORT:-18089}
base="http://127.0.0.1:$port"
name=nixmgr-smoke-$$
old=nixmgr-smoke-old-$$ new=nixmgr-smoke-new-$$
work=$(mktemp -d)
cleanup() {
  docker rm -f "$name" > /dev/null 2>&1 || true
  docker volume rm -f "$old" "$new" > /dev/null 2>&1 || true
  rm -rf "$work"
}
trap cleanup EXIT

fail() { echo "FAIL: $*" >&2; echo "--- server log" >&2; docker logs "$name" >&2 2>&1 || true; exit 1; }
json() { python3 -c "import sys, json; print(json.load(sys.stdin)$1)"; }

if [ "${SMOKE_NO_BUILD:-}" != 1 ]; then
  docker build -t "$image" "$repo"
fi
hash=$(printf 'pw\n' | docker run --rm -i "$image" hash-password) || fail "hash-password"
[[ $hash == \$2* ]] || fail "hash-password printed $hash"
echo "ok   hash-password runs through the entrypoint"

# start <volume> <mount options> [docker run options...] runs the server on the volume
# and waits for it.
start() {
  local vol=$1 opts=$2
  shift 2
  docker rm -f "$name" > /dev/null 2>&1 || true
  docker run -d --name "$name" -p "127.0.0.1:$port:8080" -v "$vol:/var/lib/nixos-manager$opts" \
    -e NIXMGR_AUTHOR_NAME="Smoke Test" -e NIXMGR_AUTHOR_EMAIL=smoke@example.invalid \
    -e NIXMGR_PASSWORD_HASH="$hash" "$@" "$image" > /dev/null
  rm -f "$work/cookies"
  for _ in $(seq 150); do
    curl -s -o /dev/null "$base/" && return
    sleep 0.2
  done
  fail "the server did not start"
}
jar=$work/cookies
api() { local m=$1 p=$2; shift 2; curl -s -b "$jar" -c "$jar" -X "$m" -H 'Content-Type: application/json' "$base$p" "$@"; }
code() { local m=$1 p=$2; shift 2; api "$m" "$p" -o /dev/null -w '%{http_code}' "$@"; }
login() { [ "$(code POST /api/login -d '{"password":"pw"}')" = 204 ] || fail "login"; }
server_uid() { docker exec "$name" cat /proc/1/status | awk '/^Uid:/ { print $2 }'; }
has_starter() { api GET /api/tree | json '["entries"]' | grep -q "'flake.nix'"; }
# not_owned lists what in the data folder, Nix's state and the home (but a mounted
# ~/.ssh) is not nixos-manager's.
not_owned() {
  docker exec "$name" find /var/lib/nixos-manager /nix/var /home/nixos-manager -path /home/nixos-manager/.ssh -prune \
    -o '(' '!' -user nixos-manager -o '!' -group nixos-manager ')' -print
}
run_job() {
  local id
  id=$(api POST /api/jobs -d "$1" | json '["id"]')
  api GET "/api/jobs/$id/events" > "$work/events-$id"
  api GET "/api/jobs/$id" | json '["state"]'
}
# ready <path> polls an endpoint that answers 202 while Nix works and prints the 200 body.
ready() {
  for _ in $(seq 600); do
    if [ "$(code GET "$1")" = 200 ]; then api GET "$1"; return; fi
    sleep 0.5
  done
  fail "$1 never became ready"
}

# The earlier image: the same server run as root, on a volume that does not start from
# the image's folder (nocopy), as the earlier image had none.
docker volume create "$old" > /dev/null
start "$old" :nocopy --user 0 --entrypoint /usr/local/bin/nixos-manager
[ "$(server_uid)" = 0 ] || fail "the old-image stand-in does not run as root"
login
has_starter || fail "the old-image stand-in did not write the starter flake"
docker rm -f "$name" > /dev/null
[ -n "$(docker run --rm --user 0 --entrypoint find -v "$old:/data" "$image" /data -user root)" ] ||
  fail "the old-image volume has no root-owned files"
echo "ok   a volume as the earlier image left it: root-owned"

# With a read-only ~/.ssh as compose.yaml suggests, owned by whoever made it.
mkdir "$work/ssh"
touch "$work/ssh/known_hosts"
start "$old" "" -v "$work/ssh:/home/nixos-manager/.ssh:ro"
uid=$(docker exec "$name" id -u nixos-manager)
if [ "$uid" = 0 ] || [ "$(server_uid)" != "$uid" ]; then
  fail "the server runs as uid $(server_uid), not nixos-manager ($uid)"
fi
[ -z "$(not_owned)" ] || fail "not handed over to nixos-manager: $(not_owned)"
login
has_starter || fail "the starter flake on the old volume is gone"
[ "$(code PUT /api/file -d '{"path":"modules/smoke.nix","content":"{ }\n","base_hash":""}')" = 200 ] ||
  fail "cannot save a file in the old volume's working copy"
echo "ok   the server runs as nixos-manager (uid $uid) and the old volume is its own"

state=$(run_job '{"kind":"lock"}')
[ "$state" = passed ] || fail "nix flake lock as nixos-manager: $state: $(cat "$work"/events-*)"
model=$(ready '/api/model?at=working')
[ "$(echo "$model" | json '["model"]["hosts"][0]["name"]')" = example ] || fail "fleet model: $model"
[ "$(echo "$model" | json '["model"].get("error", "")')" = "" ] || fail "the starter flake does not evaluate: $model"
[ -z "$(not_owned)" ] || fail "Nix left files nixos-manager does not own: $(not_owned)"
echo "ok   real Nix locks and evaluates the starter flake as nixos-manager"

# No root at all: a new volume, started as the user.
docker volume create "$new" > /dev/null
start "$new" "" --user nixos-manager
[ "$(server_uid)" = "$uid" ] || fail "--user nixos-manager runs as uid $(server_uid)"
login
has_starter || fail "the new volume did not get the starter flake"
echo "ok   a new volume works with --user nixos-manager"

echo "PASS"
