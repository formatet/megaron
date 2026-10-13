#!/usr/bin/env bash
# Same 500 weather seeds and 10,000 lanes per case; only scan semantics differ.
set -euo pipefail
TASK_ROOT="$(git -C "$(dirname "$0")/.." rev-parse --show-toplevel)"
OUT="${1:-$TASK_ROOT/docs/reviews/stormscan-varje-hex}"
mkdir -p "$OUT"
OUT="$(realpath "$OUT")"
for mode in legacy entered; do
  env -i HOME="$HOME" PATH="$PATH" go -C "$TASK_ROOT/server" test ./internal/combat -count=1 -run '^TestRO1StormSimulation$' -args \
    -storm-scan-mode="$mode" -ro1-storm-output="$OUT/storm-$mode.json"
done
python3 - "$OUT" <<'PY'
import json,pathlib,sys
p=pathlib.Path(sys.argv[1])
before=json.loads((p/'storm-legacy.json').read_text())
after=json.loads((p/'storm-entered.json').read_text())
assert {k:v for k,v in before.items() if k!='results'}=={k:v for k,v in after.items() if k!='results'}
if (p/'storm-before.json').exists():
    assert before==json.loads((p/'storm-before.json').read_text()), 'legacy arm must reproduce locked original integer scan'
rows=['| Sea | Voyage | Hexes | Before % | After % | T1 reference % |', '|---|---|---:|---:|---:|---:|']
for a,b in zip(before['results'],after['results']):
    assert (a['terrain'],a['mode'],a['steps'],a['duration_ticks'],a['journeys'])==(b['terrain'],b['mode'],b['steps'],b['duration_ticks'],b['journeys'])
    if a['steps'] not in (20,40): continue
    target=.3 if a['steps']==20 else 4.8
    rows.append(f"| {a['terrain']} | {a['mode']} | {a['steps']} | {a['percent']:.2f} | {b['percent']:.2f} | {target} |")
(p/'storm-comparison.md').write_text('\n'.join(rows)+'\n')
print('\n'.join(rows))
PY
