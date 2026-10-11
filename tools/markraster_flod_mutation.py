#!/usr/bin/env python3
"""Reintroduce each ground exemption and the old river endpoint; always restore."""
from pathlib import Path
import subprocess
import argparse
p = Path('web/static/js/megaron/render/markraster.js')
parser = argparse.ArgumentParser()
parser.add_argument('--out', default='docs/reviews/markraster-flod')
out = Path(parser.parse_args().out)
out.mkdir(parents=True, exist_ok=True)
original = p.read_bytes()
s = original.decode()
mutations = {
    'exact-ground': ('  const [eq, er] = hexAtWorld(wx, wy);\n    const [dx, dy]', '  const [eq, er] = hexAtWorld(wx, wy);\n    if (isRiverTerrain(terrainAt(eq, er))) return [eq, er];\n    const [dx, dy]'),
    'warped-ground': ('  if (t === undefined) return [eq, er];', '  if (t === undefined || isRiverTerrain(t)) return [eq, er];'),
    'mouth': ('    const p0 = [nx * reach, ny * reach]', '    const p0 = [nx * R_IN, ny * R_IN]'),
}
try:
    for name, (old, new) in mutations.items():
        assert s.count(old) == 1, name
        p.write_text(s.replace(old,new))
        r = subprocess.run(['node','--test','web/static/js/megaron/render/map.test.mjs'], capture_output=True)
        (out / f'mutation-{name}.log').write_bytes(b'\n'.join(line.rstrip() for line in (r.stdout + r.stderr).splitlines()) + b'\n')
        assert r.returncode != 0 and b'not ok' in r.stdout, name
        print(name, 'RED:', next(l.decode() for l in r.stdout.splitlines() if l.startswith(b'not ok')))
finally:
    p.write_bytes(original)
assert p.read_bytes() == original
r = subprocess.run(['node','--test','web/static/js/megaron/render/map.test.mjs'], capture_output=True)
(out / 'restored.log').write_bytes(b'\n'.join(line.rstrip() for line in (r.stdout + r.stderr).splitlines()) + b'\n')
assert r.returncode == 0
print('byte-identical restore, targeted GREEN')
