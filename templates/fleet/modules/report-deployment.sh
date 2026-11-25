# Body of the report-deployment command (report-deployments.nix sets url,
# token_file, host, state_dir, silent_after and root before it). comin runs it
# after each deployment, and in newer versions after each build, with COMIN_*
# variables describing it; a timer runs it with "heartbeat" every 15 minutes.
# Every report says which system the machine runs, boots and boots next.
# Reports that cannot be delivered wait, in order, in $state_dir/pending.jsonl;
# every later run sends them first. When the reply to the run's own report
# asks for it, the run also posts the closure of the system the machine runs
# (send_closure). It always exits 0: a lost report must never look like a
# failed deployment.

if [[ ! -r $token_file ]]; then
  exit 0 # no token on this machine: reporting is off here
fi
token=$(tr -d '[:space:]' <"$token_file") || exit 0
if [[ -z $token ]]; then
  exit 0
fi
mode=${1:-comin}
pending=$state_dir/pending.jsonl
reply=$state_dir/reply.json
running= # the system this machine runs, once snapshot has looked
closure_timeout=30 # seconds nix path-info may take before the run gives up on the closure

# post sends a report and prints the HTTP status, 000 when nothing answered;
# the reply goes to the file $2 when given. The token goes in a header read
# from a file descriptor, so it never shows in the process list.
post() {
  curl --silent --show-error --connect-timeout 5 --max-time 10 --retry 1 \
    --output "${2:-/dev/null}" --write-out '%{http_code}' \
    --header @<(printf 'Authorization: Bearer %s\n' "$token") \
    --header 'Content-Type: application/json' \
    --data-binary "$1" "${url%/}/api/deployments/report" || true
}
# refused: the manager answered that the report itself is wrong; sending it again cannot help.
refused() { [[ $1 == 400 || $1 == 413 || $1 == 415 ]]; }

# keep makes its arguments the waiting reports, the newest 20 at most.
keep() {
  if (($# == 0)); then
    rm -f "$pending"
    return 0
  fi
  local tmp
  shift $(($# > 20 ? $# - 20 : 0))
  tmp=$(mktemp "$state_dir/.pending.XXXXXX") || return 0
  if printf '%s\n' "$@" >"$tmp" && mv -f "$tmp" "$pending"; then :; else rm -f "$tmp"; fi
}
# spool adds a report after the waiting ones.
spool() {
  local lines=()
  if [[ -s $pending ]]; then
    mapfile -t lines <"$pending" || true
  fi
  keep "${lines[@]}" "$1"
}
# flush sends the waiting reports in order, and stops at the first one that is
# not delivered: it and the later ones keep waiting. True when none waits.
flush() {
  [[ -s $pending ]] || return 0
  local line code waiting=()
  while IFS= read -r line; do
    if [[ -n $line && ${#waiting[@]} -eq 0 ]]; then
      code=$(post "$line")
      if [[ $code == 2?? ]] || refused "$code"; then
        continue
      fi
    fi
    if [[ -n $line ]]; then
      waiting+=("$line")
    fi
  done <"$pending"
  keep "${waiting[@]}"
  [[ ${#waiting[@]} -eq 0 ]]
}

# snapshot sets system to what runs now. A reboot is needed when the running
# kernel, initrd or kernel modules are not the booted ones (a container has none).
link() { readlink -e "$root$1" 2>/dev/null || true; }
snapshot() {
  local current booted profile reboot=false f
  current=$(link /run/current-system)
  booted=$(link /run/booted-system)
  profile=$(link /nix/var/nix/profiles/system)
  if [[ -n $current && -n $booted ]]; then
    for f in kernel initrd kernel-modules; do
      if [[ $(link "/run/booted-system/$f") != "$(link "/run/current-system/$f")" ]]; then
        reboot=true
      fi
    done
  fi
  running=${current#"$root"}
  system=(--arg current "$running" --arg booted "${booted#"$root"}" --arg profile "${profile#"$root"}" --argjson reboot "$reboot")
}

# asked says whether the reply to the run's own report (status $1) asks for
# this machine's closure; older consoles answer 204 and never ask.
asked() { [[ $1 == 200 ]] && jq -e '.closure == "wanted"' "$reply" >/dev/null 2>&1; }

# closure_paths turns nix path-info --recursive --json into what the console
# takes: every store path with its size and the indices of the paths it
# references, itself left out. Nix before 2.19 (and Lix) prints an array of
# objects with a path; later ones an object keyed by path, null for an invalid
# path, and newer ones store paths without /nix/store/.
# shortcut: Nix 2.33 warns that --json without --json-format will become an
# error, and --json-format 2 prints {version, storeDir, info}, which this does
# not read (the tab then shows jq's error); read that shape once Nix drops 1.
closure_paths() {
  jq -c 'def full: if startswith("/") then . else "/nix/store/" + . end;
    (if type == "array" then map(select(. != null)) else to_entries | map(select(.value != null) | .value + {path: .key}) end)
    | map(.path |= full) as $ps
    | (reduce range(0; $ps | length) as $i ({}; .[$ps[$i].path] = $i)) as $ix
    | {paths: [$ps[] | . as $p | {path, narSize: (.narSize // 0),
        refs: [(.references // [])[] | full | select(. != $p.path) | $ix[.] | select(. != null)]}]}' "$1"
}

# send_closure posts the closure of the system this machine runs, or Nix's
# error when it cannot be read. Nix goes through the daemon: the heartbeat's
# file system is read-only. Both steps have a time limit, as the run holds the
# lock comin's next report waits for. A failed upload is only logged, never
# spooled: the reply to the next report asks again.
send_closure() {
  local dir code status=0
  [[ -n $running ]] || return 0
  dir=$(mktemp -d "$state_dir/.closure.XXXXXX") || return 0
  NIX_REMOTE=daemon XDG_CACHE_HOME=$state_dir/cache timeout --kill-after=5 "$closure_timeout" \
    nix --extra-experimental-features nix-command path-info --recursive --json "$running" >"$dir/raw" 2>"$dir/err" || status=$?
  if ((status == 0)); then
    closure_paths "$dir/raw" >"$dir/body" 2>>"$dir/err" || status=$?
  elif ((status == 124 || status == 137)); then
    echo "nix path-info did not finish within $closure_timeout seconds: is the Nix daemon running?" >>"$dir/err"
  fi
  if ((status != 0)); then
    [[ -s $dir/err ]] || echo "reading the closure failed (exit status $status)" >"$dir/err"
    jq -Rsc '{error: (split("\n") | map(select(. != "")) | .[-20:] | join("\n") | .[:1000])}' "$dir/err" >"$dir/body" || true
  fi
  code=000
  if [[ -s $dir/body ]]; then
    code=$(curl --silent --show-error --connect-timeout 5 --max-time 30 \
      --output /dev/null --write-out '%{http_code}' \
      --header @<(printf 'Authorization: Bearer %s\n' "$token") \
      --header 'Content-Type: application/json' \
      --data-binary @"$dir/body" \
      "${url%/}/api/deployments/closure?host=$(jq -rn --arg v "$host" '$v | @uri')&system=$(jq -rn --arg v "$running" '$v | @uri')" || true)
  fi
  rm -rf "$dir"
  if [[ $code != 2?? ]]; then
    echo "report-deployment: $url did not take this machine's closure (HTTP $code); the next report's reply asks again" >&2
  fi
  return 0
}

report() {
  if [[ $mode == heartbeat ]]; then
    # Waiting comin reports go first: they say what was deployed. While one
    # still waits, the heartbeat says the machine is up but not what it runs,
    # which the console would compare with an older deployment.
    system=(--arg current "" --arg booted "" --arg profile "" --argjson reboot false)
    if flush; then
      snapshot
    fi
    body=$(jq -nc --arg host "$host" "${system[@]}" --argjson silentAfter "$silent_after" \
      '{$host, phase: "heartbeat", $current, $booted, $profile, $reboot, $silentAfter, sendsClosure: true}') || return 0
    code=$(post "$body" "$reply")
    if [[ $code != 2?? ]]; then
      echo "report-deployment: could not report to $url (HTTP $code)" >&2
    elif asked "$code"; then
      send_closure
    fi
    rm -f "$reply"
    return 0
  fi

  phase=${COMIN_PHASE:-deploy} # comin before postBuildCommand does not set it
  status=${COMIN_STATUS:-}
  commit=${COMIN_GIT_SHA:-}
  case "$phase/$status" in
    deploy/done | deploy/failed | build/built | "build/already built" | build/failed) ;;
    *) return 0 ;;
  esac
  if [[ ! $commit =~ ^[0-9a-f]{40}$ ]]; then
    return 0
  fi
  snapshot
  body=$(jq -nc \
    --arg host "$host" --arg commit "$commit" --arg phase "$phase" --arg status "$status" \
    --arg ref "${COMIN_GIT_REF:-}" --arg generation "${COMIN_GENERATION:-}" --arg error "${COMIN_ERROR_MSG:-}" "${system[@]}" \
    '{$host, $commit, $phase, $status, ref: $ref[:200], generation: $generation[:64], error: $error[:2000], $current, $booted, $profile, $reboot, sendsClosure: true}') || return 0
  code=000
  if flush; then # in order: this report is newer than the waiting ones
    code=$(post "$body" "$reply")
  fi
  if refused "$code"; then
    echo "report-deployment: $url refused the report (HTTP $code)" >&2
  elif [[ $code != 2?? ]]; then
    spool "$body"
    echo "report-deployment: could not report to $url (HTTP $code); the next heartbeat sends it" >&2
  elif [[ $phase == deploy ]] && asked "$code"; then
    send_closure # never between a build and its deployment: that report, or the next heartbeat, sends it
  fi
  rm -f "$reply"
  return 0
}

# One run at a time: the waiting reports go out once, in order, before newer ones.
(umask 077 && mkdir -p "$state_dir") 2>/dev/null || true
if ! {
  flock -w 60 9 || true
  report
} 9>>"$state_dir/lock"; then
  report # the lock file cannot be opened: report all the same
fi
exit 0
