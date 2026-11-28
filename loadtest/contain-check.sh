#!/usr/bin/env bash
# Injected attack against the running stack (gateway :8080, responder :9092), measured end to end:
# a sensor's events are accepted; a detection in aegis-detector's exact format reaches the
# responder; we time how long until the gateway refuses that sensor, and count any that slip through.
# Prints a Markdown summary (CI appends it to the job summary). Exit 1 if containment fails.
set -u
G=${GATEWAY:-http://localhost:8080}; R=${RESPONDER:-http://localhost:9092}
ev(){ curl -s -o /dev/null -w '%{http_code}' -H 'Content-Type: application/json' -d "{\"source\":\"$1\",\"type\":\"process_start\"}" "$G/events"; }
ms(){ awk -v a="$1" -v b="$2" 'BEGIN{printf "%.2f", (b-a)/1e6}'; }
[ "$(ev edr-7)" = 202 ] || { echo "precondition failed: sensor not accepted before the attack"; exit 1; }
DET='{"rule_id":"sensor-flood","title":"Sensor event flood","severity":"high","detected_at":"'$(date -u +%FT%TZ)'","group_key":"edr-7","count":1000}'
t0=$(date +%s%N)
OUT=$(curl -s -H 'Content-Type: application/json' -d "$DET" "$R/detections"); t1=$(date +%s%N)
first=$(ev edr-7); t2=$(date +%s%N)
leaked=0; for _ in $(seq 1 200); do [ "$(ev edr-7)" = 403 ] || leaked=$((leaked+1)); done
healthy=$(ev edr-1)
echo "### Injected attack: detection -> responder -> gateway"
echo "| Measure | Value |"; echo "|---|--:|"
echo "| Responder confirmed the block | $(ms "$t0" "$t1") ms after the detection was sent |"
echo "| First hostile request refused (HTTP $first) | $(ms "$t0" "$t2") ms after the detection |"
echo "| Hostile requests accepted after containment | $leaked of 200 |"
echo "| Healthy sensor still accepted | HTTP $healthy |"
echo; echo "Responder: \`$OUT\`"
[ "$first" = 403 ] && [ "$leaked" = 0 ] && [ "$healthy" = 202 ] || { echo "**CONTAINMENT FAILED**"; exit 1; }
echo "**Contained.**"
