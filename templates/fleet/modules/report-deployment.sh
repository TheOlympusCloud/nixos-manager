# Body of the report-deployment command (report-deployments.nix sets url,
# token_file, host, state_dir, silent_after and root before it). comin runs it
# after each deployment, and in newer versions after each build, with COMIN_*
# variables describing it; a timer runs it with "heartbeat" every 15 minutes.
# Every report says which system the machine runs, boots and boots next.
# Reports that cannot be delivered wait, in order, in $state_dir/pending.jsonl;
# every later run sends them first. It always exits 0: a lost report must never
# look like a failed deployment.

if [[ ! -r $token_file ]]; then
  exit 0 # no token on this machine: reporting is off here
fi
token=$(tr -d '[:space:]' <"$token_file") || exit 0
if [[ -z $token ]]; then
  exit 0
fi
mode=${1:-comin}
pending=$state_dir/pending.jsonl

# post sends a report and prints the HTTP status, 000 when nothing answered.
# The token goes in a header read from a file descriptor, so it never shows in the process list.
post() {
  curl --silent --show-error --connect-timeout 5 --max-time 10 --retry 1 \
    --output /dev/null --write-out '%{http_code}' \
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
  system=(--arg current "${current#"$root"}" --arg booted "${booted#"$root"}" --arg profile "${profile#"$root"}" --argjson reboot "$reboot")
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
      '{$host, phase: "heartbeat", $current, $booted, $profile, $reboot, $silentAfter}') || return 0
    code=$(post "$body")
    if [[ $code != 2?? ]]; then
      echo "report-deployment: could not report to $url (HTTP $code)" >&2
    fi
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
    '{$host, $commit, $phase, $status, ref: $ref[:200], generation: $generation[:64], error: $error[:2000], $current, $booted, $profile, $reboot}') || return 0
  code=000
  if flush; then # in order: this report is newer than the waiting ones
    code=$(post "$body")
  fi
  if refused "$code"; then
    echo "report-deployment: $url refused the report (HTTP $code)" >&2
  elif [[ $code != 2?? ]]; then
    spool "$body"
    echo "report-deployment: could not report to $url (HTTP $code); the next heartbeat sends it" >&2
  fi
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
