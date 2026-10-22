#!/usr/bin/env python3
"""Override canonical physical output with stored departure; restore exactly."""
from pathlib import Path
import hashlib,subprocess,json
out=Path(__file__).resolve().parent;root=out.parents[2];target=root/'server/api/handlers/spawn_host_position.go';original=target.read_bytes()
needle=b'pos := host.Physical(ctx, db, worldID, now, at)'
assert original.count(needle)==1
cmd=['tools/gotest.sh','./api/handlers/','-run','Test(JoinSpawn|RiseSpawn)']
try:
 target.write_bytes(original.replace(needle,needle+b'\n pos.Q=host.Q;pos.R=host.R'))
 with (out/'mutation-physical-red.log').open('w') as log:r=subprocess.run(cmd,cwd=root,stdout=log,stderr=subprocess.STDOUT)
 red=r.returncode
 assert red!=0 and '--- FAIL: TestJoinSpawnUsesPhysicalHostClearance' in (out/'mutation-physical-red.log').read_text()
finally:target.write_bytes(original)
assert target.read_bytes()==original
with (out/'mutation-physical-restored-green.log').open('w') as log:r=subprocess.run(cmd,cwd=root,stdout=log,stderr=subprocess.STDOUT)
assert r.returncode==0
(out/'mutation-physical.json').write_text(json.dumps({'red_exit':red,'restored_exit':r.returncode,'restored_sha256':hashlib.sha256(original).hexdigest()},indent=2)+'\n')
