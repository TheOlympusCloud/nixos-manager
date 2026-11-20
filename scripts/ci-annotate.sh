#!/usr/bin/env bash
# ci-annotate.sh <title> <log> repeats the end of a failed CI step's output as error
# annotations: they show on the run's summary page, and unlike the log they can be read
# without signing in to GitHub. GitHub keeps about 4 kB of each, so the last 12 kB go
# out as three.
set -euo pipefail

title=$1 log=$2
dir=$(mktemp -d)
trap 'rm -rf "$dir"' EXIT
# The last 12 kB from the first whole line on, in pieces of whole lines.
if [ "$(wc -c < "$log")" -gt 11400 ]; then
  tail -c 11400 "$log" | tail -n +2 > "$dir/end"
else
  cp "$log" "$dir/end"
fi
split -C 3800 "$dir/end" "$dir/part."
# Workflow commands escape %, CR and LF in the message, and also : and , in properties.
title=${title//'%'/%25}
title=${title//:/%3A}
title=${title//,/%2C}
shopt -s nullglob
parts=("$dir"/part.*)
n=0
for p in "${parts[@]}"; do
  n=$((n + 1))
  msg=$(cat "$p")
  msg=${msg//'%'/%25}
  msg=${msg//$'\r'/%0D}
  msg=${msg//$'\n'/%0A}
  echo "::error title=$title ($n of ${#parts[@]})::$msg"
done
