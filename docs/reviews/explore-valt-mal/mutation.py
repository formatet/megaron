#!/usr/bin/env python3
"""Physically restore nearest-unseen first-leg substitution, then restore bytes."""
import os
from pathlib import Path
import subprocess
root = Path(__file__).resolve().parents[3]
source = root / 'server/internal/combat/march_start.go'
original = source.read_bytes()
needle = b'expeditionPlan = &ExpeditionPlan{AreaQ: targetQ, AreaR: targetR, LengthTicks: length}'
assert original.count(needle) == 1
env = dict(os.environ, GOTEST_PORT='57900')
args = ['tools/gotest.sh', './internal/combat/', '-run', '^TestExpedition_SelectedGalleyTargetFirstThenCoast$', '-v']
try:
    source.write_bytes(original.replace(needle, needle + b'\n\t\ttargetQ, targetR = leg.Target.Q, leg.Target.R'))
    red = subprocess.run(args, cwd=root, env=env, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
    (Path(__file__).parent / 'mutation-red.log').write_bytes(red.stdout)
    assert red.returncode != 0 and b'first leg=' in red.stdout and b'--- FAIL: TestExpedition_SelectedGalleyTargetFirstThenCoast' in red.stdout, red.stdout.decode()
finally:
    source.write_bytes(original)
assert source.read_bytes() == original
green = subprocess.run(args, cwd=root, env=env, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
(Path(__file__).parent / 'mutation-restored.log').write_bytes(green.stdout)
assert green.returncode == 0, green.stdout.decode()
print('Nearest-unseen substitution: named RED; exact-byte restoration: fresh GREEN.')
