#!/usr/bin/env bash
# Wait for a service to become healthy before continuing.
# Usage: ./scripts/wait-for-services.sh <url> [timeout_seconds]
# Example: ./scripts/wait-for-services.sh http://localhost:8080/healthz 30

set -euo pipefail

URL="${1:?Usage: $0 <health-check-url> [timeout_seconds]}"
TIMEOUT="${2:-60}"
ELAPSED=0
INTERVAL=2

echo "Waiting for ${URL} (timeout: ${TIMEOUT}s)..."

until curl -sf --max-time 3 "${URL}" > /dev/null 2>&1; do
  if [ "${ELAPSED}" -ge "${TIMEOUT}" ]; then
    echo "✗ Timed out after ${TIMEOUT}s waiting for ${URL}"
    exit 1
  fi
  sleep "${INTERVAL}"
  ELAPSED=$((ELAPSED + INTERVAL))
done

echo "✓ ${URL} is ready (${ELAPSED}s)"
