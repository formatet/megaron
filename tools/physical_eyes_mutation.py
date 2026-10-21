#!/usr/bin/env python3
"""Prove the live eye must consume the saved tick route; restore source."""
from pathlib import Path
import argparse
import subprocess

root = Path(__file__).resolve().parents[1]
p = argparse.ArgumentParser(description=__doc__)
p.add_argument('--output-dir', type=Path, required=True)
a = p.parse_args()
a.output_dir.mkdir(parents=True, exist_ok=True)
source = root / 'server/internal/province/eyes.go'
original = source.read_text()
needle = 'movement.LoadActiveRoute(row.route, status, row.departTick, row.arriveTick); ok {'
assert original.count(needle) == 1

def run(name, red):
    result = subprocess.run(['tools/gotest.sh', './api/handlers', '-run', '^TestMarchEyeUsesSavedTickRouteAtMarkerPosition$'], cwd=root, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
    (a.output_dir / (name + '.log')).write_text(result.stdout)
    if red:
        assert result.returncode and '--- FAIL: TestMarchEyeUsesSavedTickRouteAtMarkerPosition' in result.stdout and 'build failed' not in result.stdout, result.stdout
    else:
        assert result.returncode == 0, result.stdout
    print(name + (': named RED' if red else ': GREEN'), flush=True)

run('baseline', False)
try:
    source.write_text(original.replace(needle, needle.replace('; ok {', '; ok && false {')))
    run('legacy-eye', True)
finally:
    source.write_text(original)
run('restored', False)
