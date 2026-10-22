#!/usr/bin/env python3
from pathlib import Path
import hashlib,json,re
out=Path(__file__).resolve().parent;root=out.parents[2]
for line in (out/'SHA256SUMS').read_text().splitlines():
 digest,name=line.split('  ',1);assert hashlib.sha256((out/name).read_bytes()).hexdigest()==digest,name
for f,source in [('mutation-result.json','join.go'),('mutation-physical.json','spawn_host_position.go')]:
 m=json.loads((out/f).read_text());assert m['red_exit']!=0 and m['restored_exit']==0
 assert hashlib.sha256((root/'server/api/handlers'/source).read_bytes()).hexdigest()==m['restored_sha256']
assert 'four_hex_boundary_is_occupied' in (out/'mutation-red.log').read_text()
assert '--- FAIL: TestJoinSpawnUsesPhysicalHostClearance' in (out/'mutation-physical-red.log').read_text()
for name in ['targeted-rebased-green.log','mutation-restored-green.log','mutation-physical-restored-green.log']:
 s=(out/name).read_text();assert 'FAIL' not in s and 'ok ' in s,name
full=(out/'full-fresh.log').read_text();assert 'FAIL' not in full and len(re.findall(r'^ok\s',full,re.M))==26
assert 'internal/world' in full and 'migration 202' in full
assert '# fail 0' in (out/'js.log').read_text();assert not (out/'vet.log').read_text()
print('Rebased fresh DB/API, two physical RED mutations, exact restored GREEN, full Go/JS/vet and evidence hashes verified.')
