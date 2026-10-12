#!/usr/bin/env python3
"""Remove physical massif adjacency, require named red, restore and prove green."""
import pathlib
import subprocess

ROOT = pathlib.Path(__file__).resolve().parents[1]
OUT = ROOT / 'docs/reviews/flod-doparratt/massif'
SOURCE = ROOT / 'server/internal/combat/place_reach.go'
original = SOURCE.read_bytes()
try:
    old = 'if p.Kind != "massif" {'
    assert original.decode().count(old) == 1
    SOURCE.write_text(original.decode().replace(old, 'if true {', 1))
    result = subprocess.run(['tools/gotest.sh', './internal/combat/', '-run',
                             'TestPlaceReachMassifBorderEntry', '-v'],
                            cwd=ROOT, text=True, capture_output=True)
    output = result.stdout + result.stderr
    (OUT / 'mutation.log').write_text(output)
    assert result.returncode and 'massif border entry rights=0 want1' in output, output
    print('massif adjacency removed: named red')
finally:
    SOURCE.write_bytes(original)
with (OUT / 'restored.log').open('w') as log:
    subprocess.run(['tools/gotest.sh', './internal/combat/', '-run',
                    'TestPlaceReachMassifBorderEntry', '-v'], cwd=ROOT,
                   stdout=log, stderr=subprocess.STDOUT, check=True)
print('massif adjacency: restored green')
