#!/usr/bin/env bash
# Scaling experiment: submit a fixed batch of CPU-heavy jobs at pinned worker counts, measure throughput.
# Pins replicas by temporarily setting KEDA min=max so the run is controlled.
# Timing is queue-drain time: start the clock once all jobs are enqueued, stop when the
# last job reaches COMPLETED — this isolates worker processing from serial submission latency.
# Usage: BASE_URL=http://localhost:8088 BATCH=120 bash scripts/scale-test.sh
set -euo pipefail
BASE_URL="${BASE_URL:-http://localhost:8088}"
NS="image-platform"
BATCH="${BATCH:-120}"
SCALER="processing-worker-scaler"

restore_bounds() {  # always re-enable autoscaling, even if a run aborts mid-loop
  echo "Restoring autoscaling bounds (min=1,max=8)…"
  kubectl patch scaledobject "$SCALER" -n "$NS" --type=merge \
    -p '{"spec":{"minReplicaCount":1,"maxReplicaCount":8}}' >/dev/null || true
}
trap restore_bounds EXIT

pin() {  # pin worker replicas to $1 via the ScaledObject bounds
  kubectl patch scaledobject "$SCALER" -n "$NS" --type=merge \
    -p "{\"spec\":{\"minReplicaCount\":$1,\"maxReplicaCount\":$1}}" >/dev/null
  kubectl wait --for=jsonpath='{.status.readyReplicas}'=$1 deployment/processing-worker -n "$NS" --timeout=120s >/dev/null
}

upload_source() {  # upload one large image, echo its id (heavy resize work per job)
  local img id
  img=$(mktemp /tmp/scale_XXXXXX.jpg)
  # 4000×3000 = 12 MP: a real resize of this is tens-to-hundreds of ms of CPU, so the
  # worker (not submission) is the bottleneck and added replicas actually help.
  python3 -c "from PIL import Image; Image.new('RGB',(4000,3000),(120,60,30)).save('$img','JPEG',quality=95)"
  id=$(curl -sf -X POST "${BASE_URL}/api/v1/images" -F "image=@${img}" | python3 -c "import sys,json;print(json.load(sys.stdin)['id'])")
  rm -f "$img"
  echo "$id"
}

enqueue_batch() {  # create $BATCH jobs against image $1, in parallel so submission isn't the bottleneck
  local id="$1"
  seq 1 "$BATCH" | xargs -P 16 -I{} curl -sf -X POST "${BASE_URL}/api/v1/jobs" \
    -H 'Content-Type: application/json' \
    -d '{"image_id":"'"$id"'","operation":"resize","parameters":{"width":2000}}' >/dev/null
}

count_completed() {
  kubectl exec -n "$NS" deploy/postgres -- psql -U platform -d image_platform -tAc \
    "SELECT count(*) FROM jobs WHERE status='COMPLETED';"
}

echo "replicas,batch,seconds,throughput_per_s,speedup,efficiency" > /tmp/scale_results.csv
BASELINE=""
for N in 1 2 4; do
  echo "=== pinning $N worker(s) ==="
  # fresh DB state for a clean measurement (truncate children first; FK: outbox_events.job_id → jobs.id)
  kubectl exec -n "$NS" deploy/postgres -- psql -U platform -d image_platform -c \
    "TRUNCATE outbox_events, jobs RESTART IDENTITY CASCADE;" >/dev/null
  pin "$N"
  SRC=$(upload_source)
  enqueue_batch "$SRC"               # all jobs enqueued before timing starts
  START=$(python3 -c "import time;print(time.time())")
  while [ "$(count_completed | tr -d '[:space:]')" -lt "$BATCH" ]; do sleep 0.2; done
  END=$(python3 -c "import time;print(time.time())")
  SECS=$(python3 -c "print(round($END-$START,2))")
  TPUT=$(python3 -c "print(round($BATCH/($END-$START),2))")
  [ -z "$BASELINE" ] && BASELINE="$TPUT"
  SPEEDUP=$(python3 -c "print(round($TPUT/$BASELINE,2))")
  EFF=$(python3 -c "print(round($TPUT/$BASELINE/$N,2))")
  echo "$N,$BATCH,$SECS,$TPUT,$SPEEDUP,$EFF" | tee -a /tmp/scale_results.csv
done

echo ""
echo "Results CSV:"; cat /tmp/scale_results.csv
# Autoscaling bounds are restored by the EXIT trap (restore_bounds).
