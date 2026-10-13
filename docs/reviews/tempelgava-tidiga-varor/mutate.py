#!/usr/bin/env python3
"""Physical old-recipe/scarcity mutations, each on fresh migrated PG16."""
import pathlib
import subprocess

HERE = pathlib.Path(__file__).resolve().parent
ROOT = HERE.parents[2]
cases = [
    ('old-oil-wine-recipe', 'server/internal/kharis/temple_offering.go',
     '// The reference gift works even before a world\'s valuation index is ready.',
     '''if stock["oil"] >= 2 && stock["wine"] >= 1 { return map[string]float64{"oil":2,"wine":1},true }; return map[string]float64{},false
     // The reference gift works even before a world's valuation index is ready.''',
     'TestEarlyTempleOfferingLocalGiftAndProjection'),
    ('old-scarcity-recipe', 'server/internal/kharis/project.go',
     'offering := templeOfferingRecipe()', 'offering := map[string]float64{"oil":2,"wine":1}',
     'TestEarlyTempleOfferingScarcityAndDeterministicSubstitutes'),
]
for name, relative, before, after, test in cases:
    path = ROOT / relative
    original = path.read_bytes()
    source = original.decode()
    assert source.count(before) == 1
    try:
        path.write_text(source.replace(before, after))
        result = subprocess.run([str(ROOT / 'tools/gotest.sh'), './internal/kharis', '-run', '^' + test + '$', '-v'],
                                cwd=ROOT, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
        (HERE / (name + '-red.log')).write_text(result.stdout)
        assert result.returncode != 0 and '--- FAIL: ' + test in result.stdout and '[build failed]' not in result.stdout, result.stdout
    finally:
        path.write_bytes(original)
    assert path.read_bytes() == original
    result = subprocess.run([str(ROOT / 'tools/gotest.sh'), './internal/kharis', '-run', '^' + test + '$', '-v'],
                            cwd=ROOT, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
    (HERE / (name + '-restored-green.log')).write_text(result.stdout)
    assert result.returncode == 0, result.stdout
    print('PASS ' + name + ': named RED -> identical bytes -> fresh GREEN', flush=True)
