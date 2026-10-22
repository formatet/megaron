#!/usr/bin/env python3
"""Verify retained evidence and exact restored production source."""
from pathlib import Path
import hashlib,json,re
out=Path(__file__).resolve().parent;root=out.parents[2]
for line in (out/'SHA256SUMS').read_text().splitlines():
 digest,name=line.split('  ',1)
 assert hashlib.sha256((out/name).read_bytes()).hexdigest()==digest,name
assert '--- FAIL: TestRelationEverydayExactThreshold' in (out/'baseline-red.log').read_text()
assert '--- FAIL: TestRelationEverydayExactThreshold' in (out/'mutation-red.log').read_text()
assert 'FAIL' not in (out/'mutation-restored-green.log').read_text()
mutation=json.loads((out/'mutation.json').read_text())
assert mutation['byte_exact_restore'] and mutation['fresh_restored_green']
assert hashlib.sha256((root/'server/api/handlers/settlement.go').read_bytes()).hexdigest()==mutation['source_sha256']
full=(out/'full-fresh.log').read_text();assert 'FAIL' not in full and 'internal/world' in full
assert len(re.findall(r'^ok\s',full,re.M))>=25
js=(out/'js-final.log').read_text();assert '# fail 0' in js and '# cancelled 0' in js
assert not (out/'vet.log').read_text()
scenario=(out/'player-scenario.log').read_text();assert 'PASS' in scenario and '"run": 2' in scenario and '"page_errors": []' in scenario
print('Evidence hashes, baseline RED, mutant RED, exact restore GREEN, full Go, JS, vet and two client scenarios verified.')
