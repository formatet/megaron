#!/usr/bin/env python3
"""Mutate only the status consumer; preserve the actual tick's shared owner."""
from pathlib import Path
import subprocess
import sys

root = Path(__file__).resolve().parents[1]
source = root / 'server/internal/economy/population_growth.go'
original = source.read_text()
anchor = '+PopulationGrowthSQL()+'
assert original.count(anchor) == 1
base = 'ROUND(pop*0.005::float8*variety)'
mutants = {
    'variety': ('pop*0.005::float8*variety', 'pop*0.005::float8', 'variety'),
    'minimum': ('GREATEST(1,' + base + ')', base, 'minimum_one'),
    'cap': ('LEAST(pop_cap-pop,GREATEST(1,' + base + '))', 'GREATEST(1,' + base + ')', 'clamp'),
}
try:
    for name, (before, after, test_case) in mutants.items():
        replacement = '+strings.ReplaceAll(PopulationGrowthSQL(), `' + before + '`, `' + after + '`)+'
        source.write_text(original.replace(anchor, replacement))
        result = subprocess.run(['tools/gotest.sh', './api/handlers', '-run', 'TestNextCitizenStatusMatchesActualGrowthTick/' + test_case], cwd=root, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
        log = Path('/tmp/nasta-mutant-' + name + '.log')
        log.write_text(result.stdout)
        if result.returncode == 0 or 'surface growth=' not in result.stdout:
            sys.exit('Mutation did not fail the intended assertion: ' + str(log))
        print(name + ': RED, ' + str(log), flush=True)
        source.write_text(original)
finally:
    source.write_text(original)
