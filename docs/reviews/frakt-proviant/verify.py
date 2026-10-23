#!/usr/bin/env python3
from pathlib import Path
import hashlib,json,re
out=Path(__file__).resolve().parent;root=out.parents[2]
for line in (out/'SHA256SUMS').read_text().splitlines():
 digest,name=line.split('  ',1);assert hashlib.sha256((out/name).read_bytes()).hexdigest()==digest,name
m=json.loads((out/'mutation.json').read_text());assert m['red'] and m['restored_green']
assert hashlib.sha256((root/'web/static/js/megaron/ui/drawers/war.js').read_bytes()).hexdigest()==m['restored_sha256']
assert 'not ok' in (out/'baseline-red.log').read_text()
assert 'not ok' in (out/'mutation-red.log').read_text()
assert '# fail 0' in (out/'mutation-restored-green.log').read_text()
full=(out/'full-fresh.log').read_text();assert 'FAIL' not in full and len(re.findall(r'^ok\s',full,re.M))==26
assert 'migration 202' in full and 'internal/world' in full
assert '# fail 0' in (out/'js-final.log').read_text();assert not (out/'vet.log').read_text()
s=(out/'player-scenario.log').read_text();assert 'PASS' in s and s.count('"page_errors": []')==2
for i in [1,2]:
 rows=json.loads((out/f'api-run{i}.json').read_text())['units'];assert len(rows)==3 and all(u['status']=='freighting' for u in rows)
 stocks=sorted(u.get('provisions_by_good',{}).get('fish',0) for u in rows);assert stocks==[0,.05,.75]
 assert sorted(u.get('provision_days',0) for u in rows)==[0,0,7]
 cli=(out/f'cli-run{i}.txt').read_text();assert all(t in cli for t in ['food for 7 days','food for less than a day LOW','OUT OF FOOD'])
print('Actual API/CLI/browser parity, tap, old-code RED, physical mutation/byte-exact restore, full fresh Go/JS/vet and hashes verified.')
