# Body of the report-deployment command (report-deployments.nix sets url,
# token_file and host before it). comin runs it after each deployment, and in
# newer versions after each build, with COMIN_* variables describing it.
# It always exits 0: a lost report must never look like a failed deployment.

if [[ ! -r $token_file ]]; then
  exit 0 # no token on this machine: reporting is off here
fi
token=$(tr -d '[:space:]' <"$token_file") || exit 0
phase=${COMIN_PHASE:-deploy} # comin before postBuildCommand does not set it
status=${COMIN_STATUS:-}
commit=${COMIN_GIT_SHA:-}
case "$phase/$status" in
  deploy/done | deploy/failed | build/built | "build/already built" | build/failed) ;;
  *) exit 0 ;;
esac
if [[ -z $token || ! $commit =~ ^[0-9a-f]{40}$ ]]; then
  exit 0
fi

body=$(jq -n \
  --arg host "$host" --arg commit "$commit" --arg phase "$phase" --arg status "$status" \
  --arg ref "${COMIN_GIT_REF:-}" --arg generation "${COMIN_GENERATION:-}" --arg error "${COMIN_ERROR_MSG:-}" \
  '{$host, $commit, $phase, $status, ref: $ref[:200], generation: $generation[:64], error: $error[:2000]}') || exit 0

# The token goes in a header read from a file descriptor, so it never shows in the process list.
if ! curl --silent --show-error --fail --connect-timeout 5 --max-time 10 --retry 1 \
  --header @<(printf 'Authorization: Bearer %s\n' "$token") \
  --header 'Content-Type: application/json' \
  --data-binary "$body" "${url%/}/api/deployments/report" >/dev/null; then
  echo "report-deployment: could not report to $url" >&2
fi
exit 0
