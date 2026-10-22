#!/usr/bin/env python3
from pathlib import Path
import hashlib,json,re
out=Path(__file__).resolve().parent;root=out.parents[2]
for line in (out/'SHA256SUMS').read_text().splitlines():
 digest,name=line.split('  ',1);assert hashlib.sha256((out/name).read_bytes()).hexdigest()==digest,name
snapshot=json.loads((out/'live-snapshot.json').read_text());assert snapshot['transaction_read_only']=='on'
sim=json.loads((out/'simulation.json').read_text());assert sum(r['new_net']<0 for r in sim if r['cities']>0)==0
assert all(abs(r['new_burden']-(.1+.2*max(0,r['cities']-1)+.0001*r['population']))<1e-9 for r in sim)
assert 'population projection net-0.4 want-.85' in (out/'baseline-red.log').read_text()
assert '--- FAIL: TestPopulationBurdenFullTickMatchesProjection' in (out/'mutation-red.log').read_text()
assert 'FAIL' not in (out/'mutation-restored-green.log').read_text()
m=json.loads((out/'mutation.json').read_text());assert m['byte_exact_restore'] and m['restored_green']
assert hashlib.sha256((root/'server/internal/kharis/project.go').read_bytes()).hexdigest()==m['project_sha256']
full=(out/'full-fresh.log').read_text();assert 'FAIL' not in full and 'internal/world' in full
assert len(re.findall(r'^ok\s',full,re.M))>=25
assert '# fail 0' in (out/'js-final.log').read_text();assert not (out/'vet.log').read_text()
scenario=(out/'player-scenario.log').read_text();assert 'PASS' in scenario and '"run": 2' in scenario and '"page_errors": []' in scenario
for i in [1,2]:
 s=json.loads((out/f'api-run{i}.json').read_text())['settlement']
 assert s['kharis_population']==3500 and abs(s['kharis_population_decay']-.35)<1e-9 and abs(s['kharis_net_per_tick']-1.45)<1e-9
print('Read-only snapshot/simulation, evidence hashes, named RED, exact restored GREEN, full Go/JS/vet and two real client scenarios verified.')
