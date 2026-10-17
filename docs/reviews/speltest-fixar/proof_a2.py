#!/usr/bin/env python3
"""Physical tolerance/clamp mutants; each uses a fresh PG and restores bytes."""
from pathlib import Path
import hashlib
import subprocess

root = Path(__file__).resolve().parents[3]
report = Path(__file__).resolve().parent
owner = root / 'server/internal/province/goods_cost.go'
helper = root / 'server/api/handlers/helpers.go'
originals = {p: p.read_bytes() for p in (owner, helper)}
selector = 'Test(BuildTemple_FloatingPointExactCost|GoodsPayment_ResidueAndRealShortage)'

def run(name, expected):
    with (report / (name + '.log')).open('wb') as out:
        result = subprocess.run([str(root / 'tools/gotest.sh'), './api/handlers/', '-run', selector], cwd=root, stdout=out, stderr=subprocess.STDOUT)
    if not expected:
        log = (report / (name + '.log')).read_text()
        assert '--- FAIL: TestBuildTemple_FloatingPointExactCost' in log
        assert '--- FAIL: TestGoodsPayment_ResidueAndRealShortage' in log
    if (result.returncode == 0) != expected:
        raise RuntimeError(f'{name}: unexpected exit {result.returncode}')

try:
    owner.write_bytes(originals[owner].replace(b'GoodsCostEpsilon = 1e-6', b'GoodsCostEpsilon = 0'))
    run('a2-mutation-epsilon', False)
finally:
    for p, data in originals.items(): p.write_bytes(data)
try:
    for p, data in originals.items():
        p.write_bytes(data.replace(b'GREATEST(0, settled(amount, rate, calc_tick) - $1)', b'(settled(amount, rate, calc_tick) - $1)'))
    run('a2-mutation-clamp', False)
finally:
    for p, data in originals.items(): p.write_bytes(data)
run('a2-restored', True)
for p, data in originals.items():
    assert p.read_bytes() == data
    print(p.relative_to(root), hashlib.sha256(data).hexdigest(), 'restored byte-for-byte')
