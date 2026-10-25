#!/usr/bin/env bash
# Smoke test: exercises the full upload → metadata → download flow against a running API service.
# Usage: API_URL=http://localhost:8080 bash scripts/smoke-test.sh

set -euo pipefail

BASE_URL="${API_URL:-http://localhost:8080}"
JOB_URL="${JOB_SERVICE_URL:-http://localhost:8081}"
PASS=0
FAIL=0
SKIP=0

ok()   { echo "  ✓ $*"; PASS=$((PASS+1)); }
fail() { echo "  ✗ $*" >&2; FAIL=$((FAIL+1)); }
skip() { echo "  ⊘ $*"; SKIP=$((SKIP+1)); }

echo ""
echo "=== Smoke test → ${BASE_URL} ==="
echo ""

# ── 0. Health check ───────────────────────────────────────────────────────────
echo "[ 0 ] Health check"
STATUS=$(curl -s -o /dev/null -w "%{http_code}" "${BASE_URL}/healthz")
[ "${STATUS}" = "200" ] && ok "GET /healthz → 200" || { fail "GET /healthz → ${STATUS}"; exit 1; }

# ── 1. Create test image ──────────────────────────────────────────────────────
echo ""
echo "[ 1 ] Preparing test image"
TEST_IMAGE=$(mktemp /tmp/smoke_test_XXXXXX.jpg)
trap 'rm -f "${TEST_IMAGE}"' EXIT

if python3 -c "from PIL import Image; Image.new('RGB',(200,150),(180,80,40)).save('${TEST_IMAGE}','JPEG')" 2>/dev/null; then
  ok "Test JPEG created via Pillow (200×150)"
elif command -v go &>/dev/null; then
  # Fallback: generate via Go. `go run -` (stdin) is unsupported, so write the
  # program to a temp file and run that.
  GOSRC="$(mktemp -d)/mkimg.go"
  cat > "${GOSRC}" <<'GOEOF'
package main

import (
  "image"
  "image/color"
  "image/jpeg"
  "os"
)

func main() {
  img := image.NewRGBA(image.Rect(0, 0, 200, 150))
  for y := 0; y < 150; y++ {
    for x := 0; x < 200; x++ {
      img.Set(x, y, color.RGBA{R: 180, G: 80, B: 40, A: 255})
    }
  }
  f, _ := os.Create(os.Args[1])
  defer f.Close()
  jpeg.Encode(f, img, nil)
}
GOEOF
  if go run "${GOSRC}" "${TEST_IMAGE}" 2>/dev/null; then
    ok "Test JPEG created via Go (200×150)"
  else
    skip "Cannot create test image via Go"
    echo ""
    echo "Result: SKIP (0 pass, 0 fail, ${SKIP} skipped)"
    exit 0
  fi
else
  skip "Cannot create test image (install Pillow: pip install Pillow)"
  echo ""
  echo "Result: SKIP (0 pass, 0 fail, ${SKIP} skipped)"
  exit 0
fi

# ── 2. Upload ─────────────────────────────────────────────────────────────────
echo ""
echo "[ 2 ] Upload image"
UPLOAD_RESP=$(curl -sf -X POST "${BASE_URL}/api/v1/images" \
  -F "image=@${TEST_IMAGE}" 2>&1) || { fail "Upload request failed"; exit 1; }

IMAGE_ID=$(echo "${UPLOAD_RESP}" | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])" 2>/dev/null) || \
  { fail "Cannot parse image ID from: ${UPLOAD_RESP}"; exit 1; }
ok "POST /api/v1/images → 201, id=${IMAGE_ID}"

# ── 3. Metadata ───────────────────────────────────────────────────────────────
echo ""
echo "[ 3 ] Get metadata"
META=$(curl -sf "${BASE_URL}/api/v1/images/${IMAGE_ID}")
python3 - <<PYEOF
import json, sys
d = json.loads('''${META}''')
assert d["id"] == "${IMAGE_ID}", f"wrong id: {d['id']}"
assert d["content_type"] == "image/jpeg", f"wrong type: {d['content_type']}"
assert d["width"] == 200, f"wrong width: {d['width']}"
assert d["height"] == 150, f"wrong height: {d['height']}"
PYEOF
ok "GET /api/v1/images/${IMAGE_ID} → correct metadata (200×150, image/jpeg)"

# ── 4. Download redirect ──────────────────────────────────────────────────────
echo ""
echo "[ 4 ] Download redirect"
REDIR_STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
  "${BASE_URL}/api/v1/images/${IMAGE_ID}/download")
[ "${REDIR_STATUS}" = "302" ] && ok "GET .../download → 302 redirect" || \
  fail "Expected 302, got ${REDIR_STATUS}"

# ── 5. 404 for unknown image ──────────────────────────────────────────────────
echo ""
echo "[ 5 ] 404 for unknown image"
NOT_FOUND=$(curl -s -o /dev/null -w "%{http_code}" \
  "${BASE_URL}/api/v1/images/00000000-0000-0000-0000-000000000000")
[ "${NOT_FOUND}" = "404" ] && ok "GET /images/00000000-… → 404" || \
  fail "Expected 404, got ${NOT_FOUND}"

# ── 6. Invalid MIME type rejected ────────────────────────────────────────────
echo ""
echo "[ 6 ] Reject non-image file"
FAKE=$(mktemp /tmp/smoke_fake_XXXXXX.jpg)
echo "not an image" > "${FAKE}"
REJECT_STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
  -X POST "${BASE_URL}/api/v1/images" -F "image=@${FAKE}")
rm -f "${FAKE}"
[ "${REJECT_STATUS}" = "400" ] && ok "POST with text file → 400" || \
  fail "Expected 400, got ${REJECT_STATUS}"

# ── 7. Job service health check ───────────────────────────────────────────────
echo ""
echo "[ 7 ] Job service health check"
JOB_HEALTH=$(curl -s -o /dev/null -w "%{http_code}" "${JOB_URL}/healthz")
[ "${JOB_HEALTH}" = "200" ] && ok "GET ${JOB_URL}/healthz → 200" || \
  { fail "job-service not reachable (${JOB_HEALTH}) — is it running?"; }

# ── 8. Create a job ───────────────────────────────────────────────────────────
echo ""
echo "[ 8 ] Create job"
JOB_RESP=$(curl -sf -X POST "${JOB_URL}/api/v1/jobs" \
  -H "Content-Type: application/json" \
  -d '{"image_id":"'"${IMAGE_ID}"'","operation":"resize","parameters":{"width":800}}' 2>&1) || \
  { fail "Job creation request failed: ${JOB_RESP}"; }

if [ -n "${JOB_RESP}" ]; then
  JOB_ID=$(echo "${JOB_RESP}" | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])" 2>/dev/null) || \
    { fail "Cannot parse job ID from: ${JOB_RESP}"; }
  ok "POST /api/v1/jobs → 201, id=${JOB_ID}"

  # ── 9. Get job status ───────────────────────────────────────────────────────
  echo ""
  echo "[ 9 ] Get job status"
  JOB_STATUS_RESP=$(curl -sf "${JOB_URL}/api/v1/jobs/${JOB_ID}") || \
    { fail "GET /api/v1/jobs/${JOB_ID} failed"; }
  JOB_STATUS=$(echo "${JOB_STATUS_RESP}" | python3 -c "import sys,json; print(json.load(sys.stdin)['status'])" 2>/dev/null) || \
    { fail "Cannot parse status from: ${JOB_STATUS_RESP}"; }
  [ "${JOB_STATUS}" = "PENDING" ] && ok "GET /api/v1/jobs/${JOB_ID} → status=PENDING" || \
    fail "Expected status=PENDING, got ${JOB_STATUS}"

  # ── 10. 404 for unknown job ─────────────────────────────────────────────────
  echo ""
  echo "[ 10 ] 404 for unknown job"
  JOB_404=$(curl -s -o /dev/null -w "%{http_code}" \
    "${JOB_URL}/api/v1/jobs/00000000-0000-0000-0000-000000000000")
  [ "${JOB_404}" = "404" ] && ok "GET /api/v1/jobs/00000000-… → 404" || \
    fail "Expected 404, got ${JOB_404}"

  # ── 11. Invalid operation rejected ─────────────────────────────────────────
  echo ""
  echo "[ 11 ] Reject invalid operation"
  BAD_OP=$(curl -s -o /dev/null -w "%{http_code}" \
    -X POST "${JOB_URL}/api/v1/jobs" \
    -H "Content-Type: application/json" \
    -d '{"image_id":"'"${IMAGE_ID}"'","operation":"explode"}')
  [ "${BAD_OP}" = "400" ] && ok "POST with invalid operation → 400" || \
    fail "Expected 400, got ${BAD_OP}"
fi

# ── Summary ───────────────────────────────────────────────────────────────────
echo ""
echo "────────────────────────────────────────"
echo "  Pass: ${PASS}  Fail: ${FAIL}  Skip: ${SKIP}"
echo "────────────────────────────────────────"
echo ""

[ "${FAIL}" -eq 0 ] && echo "All smoke tests passed ✓" && exit 0 || exit 1
