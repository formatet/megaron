#!/usr/bin/env bash
# Kör webbklientens node-tester (alla *.test.mjs under web/). tools/gotest.sh utan
# argument anropar det här, så "hela sviten" betyder Go och webb.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
mapfile -t files < <(find web -name '*.test.mjs' | sort)
node --test "${files[@]}"
