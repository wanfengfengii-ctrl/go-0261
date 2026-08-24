#!/usr/bin/env bash
# LeafWash smoke test: builds the backend, starts it against a temporary SQLite
# data directory, and probes its local health and API behavior over HTTP.
#
# The script is fail-fast, performs no external network access, and cleans up
# every process and temporary file it creates.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT"

BIN_DIR="$(mktemp -d)"
DATA_DIR="$(mktemp -d)"
BIN="$BIN_DIR/leafwash"
PORT="${LEAFWASH_SMOKE_PORT:-18080}"
ADDR="127.0.0.1:${PORT}"
PID=""

cleanup() {
  if [[ -n "$PID" ]]; then
    kill "$PID" 2>/dev/null || true
    wait "$PID" 2>/dev/null || true
  fi
  rm -f "$BIN"
  rm -f "$DATA_DIR/leafwash.db" "$DATA_DIR/leafwash.db-wal" "$DATA_DIR/leafwash.db-shm" 2>/dev/null || true
  rmdir "$BIN_DIR" 2>/dev/null || true
  rmdir "$DATA_DIR" 2>/dev/null || true
}
trap cleanup EXIT

echo "==> building backend"
go build -o "$BIN" ./cmd/leafwash

echo "==> starting backend on $ADDR"
LEAFWASH_ADDR="$ADDR" LEAFWASH_DATA="$DATA_DIR/leafwash.db" "$BIN" &
PID=$!

# Wait for the health endpoint to come up (bounded, deterministic).
ready=0
for _ in $(seq 1 100); do
  if curl -sf "http://$ADDR/healthz" >/dev/null 2>&1; then
    ready=1
    break
  fi
  sleep 0.1
done
if [[ "$ready" -ne 1 ]]; then
  echo "FAIL: backend did not become ready" >&2
  exit 1
fi

# Capture responses in variables (never pipe curl into grep) and assert them.
health="$(curl -s "http://$ADDR/healthz")"
if [[ "$health" != *'"status":"ok"'* ]]; then
  echo "FAIL: health probe returned: $health" >&2
  exit 1
fi

lock_body='{"task_id":"TASK-SMOKE","base_lot_id":"BL-2026-001","seal_id":"SEAL-001","precool_lot":"PC-001","cut_line_id":"CUT-3","wash_tank_id":"TANK-A","formula_id":"F-100","formula_revision":3,"sample_times":[0,300,600],"blind_codes":["BLIND-01","BLIND-02","BLIND-03"],"atp_points":["ATP-1","ATP-2","ATP-3"],"plate_wells":["WELL-A1","WELL-A2","WELL-B1"],"drain_slots":["DRAIN-1"],"reviewers":["P-2001","P-2002"]}'

lock_resp="$(curl -s -X POST "http://$ADDR/api/tasks/lock" -H 'Content-Type: application/json' -d "$lock_body")"
if [[ "$lock_resp" != *'"state":"pending_feed"'* ]]; then
  echo "FAIL: lock returned: $lock_resp" >&2
  exit 1
fi

task_resp="$(curl -s "http://$ADDR/api/tasks/TASK-SMOKE")"
if [[ "$task_resp" != *'"task_id":"TASK-SMOKE"'* ]]; then
  echo "FAIL: get task returned: $task_resp" >&2
  exit 1
fi

echo "smoke OK: health=${health}"
