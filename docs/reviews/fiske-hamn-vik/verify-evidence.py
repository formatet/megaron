#!/usr/bin/env python3
"""Check saved fishing gate outcomes; never connects to game data."""
from pathlib import Path
p=Path(__file__).resolve().parent
assert 'FAIL: TestDeepFishingRequiresHarbour' in (p/'baseline-red.log').read_text()
red=(p/'mutation-red.log').read_text()
for name in ['TestBayLoadedOwnerCountsRiversButNotMissingMap','TestBayFishingHTTPUsesLocalCapacity']:
    assert 'FAIL: '+name in red
restored=(p/'mutation-restored-green.log').read_text()
assert 'FAIL' not in restored and sum(line.startswith('ok') for line in restored.splitlines())==2
assert '# fail 0' in (p/'js.log').read_text()
assert not (p/'vet.log').read_text().strip()
full=(p/'full-fresh.log').read_text()
assert 'FAIL' not in full and 'internal/world' in full
print('Saved named RED, mutation RED/restore GREEN, JS/vet/full Go evidence verified')
