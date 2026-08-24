#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
OUT_DIR="${1:-$ROOT/docs/release-evidence/ux-dogfood-$STAMP}"
BASE_URL="${WAYPOINT_BASE_URL:-http://127.0.0.1:8080}"

mkdir -p "$OUT_DIR"

if ! docker info >/dev/null 2>&1; then
  printf 'BLOCKER: Docker daemon unavailable; start the Waypoint compose stack on a host with Docker, PostgreSQL, and Chromium/Playwright.\n' | tee "$OUT_DIR/blocker.txt" >&2
  exit 1
fi

cleanup() {
  if [[ "${KEEP_WAYPOINT_STACK:-0}" != "1" ]]; then
    docker compose down -v >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

printf 'Starting Waypoint compose stack…\n'
docker compose up -d --build

printf 'Waiting for readiness at %s/readyz…\n' "$BASE_URL"
for _ in $(seq 1 90); do
  if curl -fsS "$BASE_URL/readyz" >/dev/null; then
    break
  fi
  sleep 2
done
curl -fsS "$BASE_URL/readyz" | tee "$OUT_DIR/readyz.json"

node "$ROOT/scripts/ux-dogfood-browser.mjs" \
  --base-url "$BASE_URL" \
  --out-dir "$OUT_DIR"

printf 'Evidence written to %s\n' "$OUT_DIR"
