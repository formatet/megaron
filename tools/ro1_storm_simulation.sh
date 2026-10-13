#!/usr/bin/env bash
# Paired synthetic storm measurement against the locked pre-RÖ1 implementation.
set -euo pipefail
TASK_ROOT="$(git -C "$(dirname "$0")/.." rev-parse --show-toplevel)"
OUT="${1:-$TASK_ROOT/docs/reviews/rorelsetabellen}"
mkdir -p "$OUT"
OUT="$(realpath "$OUT")"
SIM_BASE="$(mktemp -d /tmp/megaron-ro1-storm-baseline-XXXXXX)"
cleanup() { git -C "$TASK_ROOT" worktree remove --force "$SIM_BASE" >/dev/null; }
trap cleanup EXIT
git -C "$TASK_ROOT" worktree add --detach "$SIM_BASE" b0762ebf >/dev/null
cp "$TASK_ROOT/server/internal/combat/ro1_storm_simulation_test.go" "$SIM_BASE/server/internal/combat/"
cp "$TASK_ROOT/server/internal/combat/storm_encounters.go" "$SIM_BASE/server/internal/combat/"
env -u DATABASE_URL -u TICK_SECONDS go -C "$SIM_BASE/server" test ./internal/combat -count=1 -run '^TestRO1StormSimulation$' -args -ro1-storm-output="$OUT/storm-before.json"
env -u DATABASE_URL -u TICK_SECONDS go -C "$TASK_ROOT/server" test ./internal/combat -count=1 -run '^TestRO1StormSimulation$' -args -ro1-storm-output="$OUT/storm-after.json"
