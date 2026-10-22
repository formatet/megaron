#!/usr/bin/env python3
"""Verify saved final payroll gates, without DB access."""
from pathlib import Path
p=Path(__file__).resolve().parent
assert 'FAIL: TestSoldierWagesUseActualMenAndHomeHalf' in (p/'baseline-red.log').read_text()
r=(p/'mutation-red.log').read_text()
assert 'FAIL: TestSoldierWagesUseActualMenAndHomeHalf' in r
assert 'FAIL: TestWagesChargeActualPeopleWithoutCirculation' in r
restored=(p/'mutation-restored-green.log').read_text()
assert 'FAIL' not in restored and sum(l.startswith('ok') for l in restored.splitlines())==2
assert '# tests 715' in (p/'js-final.log').read_text() and '# fail 0' in (p/'js-final.log').read_text()
assert not (p/'vet-final.log').read_text().strip()
full=(p/'full-fresh.log').read_text()
assert 'FAIL' not in full and 'internal/world' in full
print('Payroll named RED, physical mutation RED/exact restore GREEN,715JS/vet/full fresh Go verified')
