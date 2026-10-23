#!/usr/bin/env bash
# Five isolated acceptance fixtures; never accepts a production URL or DSN.
set -euo pipefail
TASK_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
exec python3 "$TASK_ROOT/tools/kharis_texttest.py" "$@"
