#!/usr/bin/env bash
# Goop2 Cluster Executor — minimal shell example
#
# Usage: ./executor-bash.sh [goop2-url]
#
# Polls for jobs, claims them, does fake work, reports result.
# Replace the "do work" section with your actual processing.

set -euo pipefail

BASE="${1:-http://localhost:8787}"
POLL_INTERVAL=2

log() { echo "[executor] $(date +%H:%M:%S) $*"; }

# Heartbeat in background
heartbeat() {
    while true; do
        curl -sf -X POST "$BASE/api/cluster/heartbeat" \
            -H 'Content-Type: application/json' \
            -d '{"stats":{"executor":"bash","pid":'$$'}}' >/dev/null 2>&1 || true
        sleep 10
    done
}
heartbeat &
HEARTBEAT_PID=$!
trap "kill $HEARTBEAT_PID 2>/dev/null" EXIT

log "polling $BASE for jobs..."

while true; do
    # 1. Poll for pending jobs
    JOBS=$(curl -sf "$BASE/api/cluster/job" 2>/dev/null || echo '{"pending":[]}')
    PENDING=$(echo "$JOBS" | python3 -c "import sys,json; print(len(json.load(sys.stdin).get('pending',[])))" 2>/dev/null || echo 0)

    if [ "$PENDING" = "0" ]; then
        sleep "$POLL_INTERVAL"
        continue
    fi

    # Grab first pending job
    JOB_ID=$(echo "$JOBS" | python3 -c "import sys,json; print(json.load(sys.stdin)['pending'][0]['job']['id'])")
    JOB_TYPE=$(echo "$JOBS" | python3 -c "import sys,json; print(json.load(sys.stdin)['pending'][0]['job']['type'])")
    log "found job $JOB_ID (type=$JOB_TYPE)"

    # 2. Accept
    ACCEPT=$(curl -sf -X POST "$BASE/api/cluster/accept" \
        -H 'Content-Type: application/json' \
        -d "{\"job_id\":\"$JOB_ID\"}")
    log "accepted: $ACCEPT"

    # 3. Do work (replace this with your actual processing)
    for pct in 25 50 75; do
        sleep 1
        curl -sf -X POST "$BASE/api/cluster/progress" \
            -H 'Content-Type: application/json' \
            -d "{\"job_id\":\"$JOB_ID\",\"percent\":$pct,\"message\":\"processing ($pct%)\"}" >/dev/null
        log "progress: $pct%"
    done

    # 4. Report result
    curl -sf -X POST "$BASE/api/cluster/result" \
        -H 'Content-Type: application/json' \
        -d "{\"job_id\":\"$JOB_ID\",\"success\":true,\"result\":{\"executor\":\"bash\",\"type\":\"$JOB_TYPE\"}}"
    log "completed $JOB_ID"
done
