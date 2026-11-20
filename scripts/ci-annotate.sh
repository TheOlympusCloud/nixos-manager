#!/usr/bin/env bash
# ci-annotate.sh <title> <log> repeats the end of a failed CI step's output as an error
# annotation: it shows on the run's summary page, and unlike the log it can be read
# without signing in to GitHub.
set -euo pipefail

title=$1 log=$2
# The last 12 kB, from the first whole line on.
if [ "$(wc -c < "$log")" -gt 12000 ]; then
  msg=$(tail -c 12000 "$log" | tail -n +2)
else
  msg=$(cat "$log")
fi
# Workflow commands escape %, CR and LF in the message, and also : and , in properties.
msg=${msg//'%'/%25}
msg=${msg//$'\r'/%0D}
msg=${msg//$'\n'/%0A}
title=${title//'%'/%25}
title=${title//:/%3A}
title=${title//,/%2C}
echo "::error title=$title::$msg"
