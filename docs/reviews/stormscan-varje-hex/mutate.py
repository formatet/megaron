#!/usr/bin/env python3
"""Physical tick-only/repeated-storm mutations, each on fresh migrated PG16."""
import pathlib
import subprocess

HERE = pathlib.Path(__file__).resolve().parent
ROOT = HERE.parents[2]
cases = [
 ('tick-only','server/internal/combat/storm_encounters.go',
  'for i := first; i < len(enter) && enter[i] <= end; i++ {\n\t\t\tvisit(i)\n\t\t}',
  'if index:=holding(t); index>=1 { visit(index) }','TestStormEnteredHexesActualScan'),
 ('same-storm-repeated','server/internal/combat/storm_encounters.go',
  'id != uuid.Nil && id != previous','id != uuid.Nil && (id != previous || id == previous)','TestStormEnteredHexesActualScan'),
]
for name, relative, before, after, test in cases:
    path = ROOT / relative
    original = path.read_bytes()
    source = original.decode()
    assert source.count(before) == 1
    try:
        path.write_text(source.replace(before, after))
        result = subprocess.run([str(ROOT / 'tools/gotest.sh'), './internal/combat', '-run', '^' + test + '$', '-v'],
                                cwd=ROOT, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
        (HERE / (name + '-red.log')).write_text(result.stdout)
        assert result.returncode != 0 and '--- FAIL: ' + test in result.stdout and '[build failed]' not in result.stdout, result.stdout
    finally:
        path.write_bytes(original)
    assert path.read_bytes() == original
    result = subprocess.run([str(ROOT / 'tools/gotest.sh'), './internal/combat', '-run', '^' + test + '$', '-v'],
                            cwd=ROOT, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
    (HERE / (name + '-restored-green.log')).write_text(result.stdout)
    assert result.returncode == 0, result.stdout
    print('PASS ' + name + ': named RED -> identical bytes -> fresh GREEN', flush=True)
