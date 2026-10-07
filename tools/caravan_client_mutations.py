#!/usr/bin/env python3
"""Physical client/map mutations. Run after production edits freeze.
Each mutated named test must fail, then the restored source must pass on a
new database (Go), or a fresh Node process (JS). No production environments.
"""
from pathlib import Path
import subprocess
import sys

root = Path(__file__).resolve().parents[1]
out = Path(sys.argv[1]).resolve()
out.mkdir(parents=True, exist_ok=True)
mutations = [
 ('map-position', 'server/api/handlers/world.go',
  'm.CurrentQ, m.CurrentR = &pos.Q, &pos.R',
  'wrongQ := pos.Q + 1\n\t\t\tm.CurrentQ, m.CurrentR = &wrongQ, &pos.R',
  ['tools/gotest.sh', './api/handlers', '-run', '^TestMapTradesFrozenPositionAndArrival$']),
 ('cli-arrival', 'server/cmd/keryx/output.go',
  'arrival, duration)', 'arrival+1, duration)',
  ['tools/gotest.sh', './cmd/keryx', '-run', '^TestTransferDisplaysServerTicksAndShip$']),
 ('web-arrival', 'web/static/js/megaron/ui/drawers/diplomacy.js',
  "'tick ' + data.goods_arrival_tick :", "'tick ' + (data.goods_arrival_tick + 1) :",
  ['node', '--test', 'web/static/js/megaron/ui/drawers/trade_schedule.test.mjs']),
]
for name, path, original, mutated, command in mutations:
 target = root / path
 before = target.read_bytes()
 text = before.decode()
 assert text.count(original) == 1, (name, 'mutation target changed')
 try:
  target.write_text(text.replace(original, mutated))
  with (out/(name+'-red.log')).open('w') as log:
   red = subprocess.run(command, cwd=root, stdout=log, stderr=subprocess.STDOUT)
  assert red.returncode != 0, (name, 'mutation survived')
 finally:
  target.write_bytes(before)
 with (out/(name+'-restored.log')).open('w') as log:
  green = subprocess.run(command, cwd=root, stdout=log, stderr=subprocess.STDOUT)
 assert green.returncode == 0, (name, 'restored source failed')
 print(name + ': red -> restored green', flush=True)
