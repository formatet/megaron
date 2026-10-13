#!/usr/bin/env python3
"""Physical old-negative-L1 yield mutation, each on fresh migrated PG16."""
import pathlib
import subprocess

HERE = pathlib.Path(__file__).resolve().parent
ROOT = HERE.parents[2]
cases = [('old-eight-yield','server/internal/kharis/tick.go','const kharisPerTempleDay = 14.0','const kharisPerTempleDay = 8.0','TestL1TempleClimbsToFirstPrayerAndCaps')]
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
