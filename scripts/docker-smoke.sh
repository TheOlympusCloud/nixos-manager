#!/usr/bin/env bash
# Builds the Docker image with compose.yaml and checks that the server's container runs
# nothing as root:
# - a volume written by the earlier image (which ran as root) is given to the user by
#   Compose's data-owner and keeps working;
# - real Nix locks and evaluates the starter flake as the user;
# - a program the user swaps into its Nix profile never runs as root, even after a restart;
# - a new volume works with plain docker run.
#
# Needs: docker with Compose, curl, python3, and network (the image build and the flake
# inputs). With GITHUB_TOKEN set, Nix in the container uses it for the starter flake's
# github: inputs (anonymous calls to GitHub's API are rate limited, and CI runners share
# addresses).
set -euo pipefail

repo=$(cd "$(dirname "$0")/.." && pwd)
port=${SMOKE_PORT:-18089}
base="http://127.0.0.1:$port"
proj=nixmgr-smoke-$$
image=$proj-nixos-manager # Compose's name for the service's image
vol=${proj}_nixmgr-data new=$proj-new plain=$proj-plain
work=$(mktemp -d)
compose() { docker compose -p "$proj" -f "$repo/compose.yaml" -f "$work/override.yaml" "$@"; }
cleanup() {
  compose down -v --rmi local > /dev/null 2>&1 || true
  docker rm -f "$plain" > /dev/null 2>&1 || true
  docker volume rm -f "$vol" "$new" > /dev/null 2>&1 || true
  rm -rf "$work"
}
trap cleanup EXIT
cid="" # the container whose log a failure shows
fail() { echo "FAIL: $*" >&2; echo "--- server log" >&2; [ -z "$cid" ] || docker logs "$cid" >&2 2>&1 || true; exit 1; }
json() { python3 -c "import sys, json; print(json.load(sys.stdin)$1)"; }

mkdir "$work/secrets"
cat > "$work/override.yaml" << EOF
services:
  nixos-manager:
    ports: !override ["127.0.0.1:$port:8080"]
    environment:
      NIXMGR_AUTHOR_NAME: Smoke Test
      NIXMGR_AUTHOR_EMAIL: smoke@example.invalid
      NIX_CONFIG: "${GITHUB_TOKEN:+access-tokens = github.com=$GITHUB_TOKEN}"
    volumes:
      - $work/secrets:/run/secrets/nixmgr:ro
EOF
compose build
hash=$(printf 'pw\n' | docker run --rm -i "$image" hash-password) || fail "hash-password"
[[ $hash == \$2* ]] || fail "hash-password printed $hash"

up() { # waits for the server on $port
  for _ in $(seq 150); do
    curl -s -o /dev/null "$base/" && return
    sleep 0.2
  done
  fail "the server did not start"
}
jar=$work/cookies
api() { local m=$1 p=$2; shift 2; curl -s -b "$jar" -c "$jar" -X "$m" -H 'Content-Type: application/json' "$base$p" "$@"; }
code() { local m=$1 p=$2; shift 2; api "$m" "$p" -o /dev/null -w '%{http_code}' "$@"; }
login() { rm -f "$jar"; [ "$(code POST /api/login -d '{"password":"pw"}')" = 204 ] || fail "login"; }
uids() { docker exec "$cid" cat /proc/1/status | awk '/^Uid:/ { print $2, $3, $4, $5 }'; }
has_starter() { api GET /api/tree | json '["entries"]' | grep -q "'flake.nix'"; }
# not_owned lists what in the data folder, Nix's state and the home is not nixos-manager's.
not_owned() { docker exec "$cid" find /var/lib/nixos-manager /nix/var /home/nixos-manager '(' '!' -user nixos-manager -o '!' -group nixos-manager ')'; }
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

# The earlier image: the same server run as root, on the volume Compose will use. nocopy:
# the volume does not take this image's folder, which the user owns; the earlier one was root's.
docker volume create "$vol" > /dev/null
cid=$(docker run -d -p "127.0.0.1:$port:8080" -v "$vol:/var/lib/nixos-manager:nocopy" --user 0 \
  -e NIXMGR_AUTHOR_NAME=Old -e NIXMGR_AUTHOR_EMAIL=old@example.invalid -e NIXMGR_PASSWORD_HASH="$hash" "$image")
up
[ "$(uids)" = "0 0 0 0" ] || fail "the old-image stand-in does not run as root"
login
has_starter || fail "the old-image stand-in did not write the starter flake"
docker rm -f "$cid" > /dev/null
[ -n "$(docker run --rm --user 0 --entrypoint find -v "$vol:/data" "$image" /data -user root)" ] ||
  fail "the old-image volume has no root-owned files"
echo "ok   a volume as the earlier image left it: root-owned"

# The README's steps: the hash through Compose, then up.
printf 'pw\n' | compose run --rm -T nixos-manager hash-password > "$work/secrets/password-hash" ||
  fail "compose run hash-password"
compose up -d
cid=$(compose ps -q nixos-manager)
up
uid=$(docker exec "$cid" id -u)
[ "$uid" = 10001 ] || fail "docker exec runs as uid $uid, not the user"
[ "$(uids)" = "10001 10001 10001 10001" ] || fail "the server runs as uids $(uids)"
[ -z "$(not_owned)" ] || fail "not given to nixos-manager: $(not_owned)"
login
has_starter || fail "the starter flake on the old volume is gone"
[ "$(code PUT /api/file -d '{"path":"modules/smoke.nix","content":"{ }\n","base_hash":""}')" = 200 ] ||
  fail "cannot save a file in the old volume's working copy"
echo "ok   data-owner gave the old volume to nixos-manager, and the server runs as it"

state=$(run_job '{"kind":"lock"}')
[ "$state" = passed ] || fail "nix flake lock as nixos-manager: $state: $(cat "$work"/events-*)"
model=$(ready '/api/model?at=working')
[ "$(echo "$model" | json '["model"]["hosts"][0]["name"]')" = example ] || fail "fleet model: $model"
[ "$(echo "$model" | json '["model"].get("error", "")')" = "" ] || fail "the starter flake does not evaluate: $model"
[ "$(echo "$model" | json '["model"]["hosts"][0].get("error", "")')" = "" ] || fail "the example machine does not evaluate: $model"
[ -z "$(not_owned)" ] || fail "Nix left files nixos-manager does not own: $(not_owned)"
echo "ok   real Nix locks and evaluates the starter flake as nixos-manager"

secrets=$(api GET /api/secrets)
[ "$(echo "$secrets" | json '["missing"]')" = "[]" ] || fail "the image lacks a tool the secrets need: $secrets"
case "$(echo "$secrets" | json '["recipient"]')" in age1*) ;; *) fail "no age key for the secrets: $secrets" ;; esac
[ -z "$(not_owned)" ] || fail "the age key is not nixos-manager's: $(not_owned)"
echo "ok   sops, age and ssh-to-age are in the image, and the server made its age key"

# The user swaps both Nix profiles for wrappers that log who runs them, then the stack
# restarts with the container's files kept: nothing may run them as root. One log per uid
# ($EUID: bash is /bin/sh, and id would be a wrapper too), as the kernel's protected_regular
# stops root from writing to the user's file in /tmp.
docker exec "$cid" sh -c '
  set -e
  for p in default tools; do
    real=$(readlink -f /nix/var/nix/profiles/$p)
    mkdir -p /tmp/swap/$p/bin
    for t in "$real"/bin/*; do
      printf "#!/bin/sh\necho %s >> /tmp/ran-\$EUID\nexec %s \"\$@\"\n" "${t##*/}" "$t" > /tmp/swap/$p/bin/${t##*/}
      chmod +x /tmp/swap/$p/bin/${t##*/}
    done
    ln -sfn /tmp/swap/$p /nix/var/nix/profiles/$p
  done'
compose stop
compose up -d
[ "$(compose ps -q nixos-manager)" = "$cid" ] || fail "the server's container was replaced, not restarted"
up
login
has_starter || fail "no starter flake after the restart"
docker exec "$cid" test -s /tmp/ran-10001 || fail "the swapped profile never ran, so this check proves nothing"
if docker exec "$cid" test -e /tmp/ran-0; then
  fail "root ran programs the user controls: $(docker exec "$cid" sort -u /tmp/ran-0 | tr '\n' ' ')"
fi
echo "ok   programs swapped into the user's profile never run as root"
compose down

# A new volume with plain docker run: nothing gives it to the user but the image.
cid=$(docker run -d --name "$plain" -p "127.0.0.1:$port:8080" -v "$new:/var/lib/nixos-manager" \
  -e NIXMGR_AUTHOR_NAME=New -e NIXMGR_AUTHOR_EMAIL=new@example.invalid -e NIXMGR_PASSWORD_HASH="$hash" "$image")
up
[ "$(uids)" = "10001 10001 10001 10001" ] || fail "docker run runs the server as uids $(uids)"
login
has_starter || fail "the new volume did not get the starter flake"
echo "ok   a new volume works with plain docker run"

echo "PASS"
