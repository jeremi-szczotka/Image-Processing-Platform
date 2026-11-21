#!/usr/bin/env bash
# K8s end-to-end smoke test — everything goes through the Ingress (frontend nginx routes internally).
# Usage: BASE_URL=http://localhost:8088 bash scripts/k8s-smoke.sh
set -euo pipefail
BASE_URL="${BASE_URL:-http://localhost:8088}"
ok(){ echo "  ✓ $*"; }; fail(){ echo "  ✗ $*" >&2; exit 1; }

echo "=== K8s smoke → ${BASE_URL} ==="

# 0. health (nginx → api-service)
[ "$(curl -s -o /dev/null -w '%{http_code}' "${BASE_URL}/healthz")" = "200" ] \
  && ok "healthz 200" || fail "healthz not 200"

# 1. make a test JPEG
IMG=$(mktemp /tmp/k8s_smoke_XXXXXX.jpg); trap 'rm -f "$IMG"' EXIT
python3 -c "from PIL import Image; Image.new('RGB',(200,150),(180,80,40)).save('$IMG','JPEG')"

# 2. upload → image id
UP=$(curl -sf -X POST "${BASE_URL}/api/v1/images" -F "image=@${IMG}")
IMAGE_ID=$(echo "$UP" | python3 -c "import sys,json;print(json.load(sys.stdin)['id'])")
ok "uploaded image ${IMAGE_ID}"

# 3. create job
JOB=$(curl -sf -X POST "${BASE_URL}/api/v1/jobs" -H 'Content-Type: application/json' \
  -d '{"image_id":"'"$IMAGE_ID"'","operation":"resize","parameters":{"width":800}}')
JOB_ID=$(echo "$JOB" | python3 -c "import sys,json;print(json.load(sys.stdin)['id'])")
ok "created job ${JOB_ID}"

# 4. poll for COMPLETED (worker must process it → proves queue + worker + buckets)
for i in $(seq 1 30); do
  ST=$(curl -sf "${BASE_URL}/api/v1/jobs/${JOB_ID}" | python3 -c "import sys,json;print(json.load(sys.stdin)['status'])")
  [ "$ST" = "COMPLETED" ] && break
  [ "$ST" = "FAILED" ] && fail "job FAILED"
  sleep 1
done
[ "$ST" = "COMPLETED" ] || fail "job not COMPLETED (last=$ST)"
ok "job COMPLETED"

# 5. download the RESULT via the /result route (the gap the old smoke test never asserted)
RST=$(curl -s -o /tmp/k8s_result.out -w '%{http_code}' "${BASE_URL}/api/v1/jobs/${JOB_ID}/result")
[ "$RST" = "200" ] || fail "result GET not 200 (got $RST)"
[ -s /tmp/k8s_result.out ] || fail "result body empty"
ok "result downloaded 200, $(wc -c </tmp/k8s_result.out) bytes"
rm -f /tmp/k8s_result.out

echo "=== PASS ==="
