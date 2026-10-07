#!/usr/bin/env python3
"""Run temporary growth-gate mutations in this isolated worktree; restore always."""
import pathlib
import subprocess
import sys

root = pathlib.Path(__file__).resolve().parents[3]
source = root / 'server/internal/kharis/tick.go'
original = source.read_text()
out = pathlib.Path(sys.argv[1] if len(sys.argv) > 1 else '/tmp/megaron-foodnet-mutations')
out.mkdir(parents=True, exist_ok=True)
mutations = {
    'grain-only': ('economy.NetFoodGoods, growthRatePerTick,', '[]string{economy.GoodGrain}, growthRatePerTick,'),
    'nonnegative': ('WHEN food_net > 0 THEN pop', 'WHEN food_net >= 0 THEN pop'),
    'fed-always': ('WHEN food_net > 0 THEN pop', 'WHEN unmet = 0 THEN pop'),
}

def run(name):
    with (out / (name + '.log')).open('w') as log:
        result = subprocess.run([str(root / 'tools/gotest.sh'), './internal/kharis/', '-run', '^TestApplyDecay_GrowthGateMatchesFoodNet$'], cwd=root, stdout=log, stderr=subprocess.STDOUT)
    return result.returncode, (out / (name + '.log')).read_text()

for name, (before, after) in mutations.items():
    assert original.count(before) == 1, (name, 'mutation anchor drift')
    try:
        source.write_text(original.replace(before, after))
        code, log = run(name)
        assert code != 0 and '--- FAIL: TestApplyDecay_GrowthGateMatchesFoodNet/' in log and ('did not grow' in log or 'want hold' in log), (name, 'not an assertion failure; inspect log')
    finally:
        source.write_text(original)
    code, log = run(name + '-restored')
    assert code == 0 and 'ok  ' in log, (name, 'restored failed')
    print(name + ': assertion red -> restored green', flush=True)
