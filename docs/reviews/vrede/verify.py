#!/usr/bin/env python3
from pathlib import Path
import hashlib,json,re
out=Path(__file__).resolve().parent;root=out.parents[2]
for line in (out/'SHA256SUMS').read_text().splitlines():
 digest,name=line.split('  ',1);assert hashlib.sha256((out/name).read_bytes()).hexdigest()==digest,name
m=json.loads((out/'mutation.json').read_text());assert m['restored_green']
for name,digest in m['byte_exact_restore'].items():
 assert hashlib.sha256((root/name).read_bytes()).hexdigest()==digest,name
for name in ['shield','positive-net']:
 assert '--- FAIL: TestWrath' in (out/f'mutation-{name}-red.log').read_text()
assert 'FAIL' not in (out/'mutation-restored-green.log').read_text()
assert '--- FAIL: TestWrath' in (out/'baseline-red.log').read_text()
full=(out/'full-fresh.log').read_text();assert 'FAIL' not in full and 'internal/world' in full
assert len(re.findall(r'^ok\s',full,re.M))>=25
assert '# fail 0' in (out/'js-final.log').read_text();assert not (out/'vet.log').read_text()
scenario=(out/'player-scenario.log').read_text();assert 'PASS' in scenario and '"run": 2' in scenario
assert scenario.count('"page_errors": []')==2
for i in [1,2]:
 s=json.loads((out/f'api-run{i}.json').read_text())['settlement']
 assert s['kharis_wrath']['protection_remaining_days']==7
 assert s['kharis_wrath']['protection_until']
 assert '23 days' in (out/f'cli-notice-run{i}.txt').read_text()
print('Evidence hashes, two physical RED mutations, byte-exact restored GREEN, full fresh Go/JS/vet and two real client scenarios verified.')
