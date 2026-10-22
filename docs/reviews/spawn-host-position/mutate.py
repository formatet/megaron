#!/usr/bin/env python3
"""Physically mutate the Host clearance boundary; fresh DB for each arm."""
from pathlib import Path
import hashlib,os,subprocess,json
p=Path(__file__).resolve().parent;root=p.parents[2];target=root/'server/api/handlers/join.go';original=target.read_bytes()
needle=b'ABS((mt.q + mt.r) - (h.q + h.r))) / 2 <= 4'
assert original.count(needle)==1
env=os.environ.copy();env['GOTEST_PORT']='57900'
command=['tools/gotest.sh','./api/handlers/','-run','Test(JoinSpawn|RiseSpawn|Join_Balances|Rise_Refugee|HostPhysical)']
try:
 target.write_bytes(original.replace(needle,needle.replace(b'<= 4',b'<= 3')))
 with (p/'mutation-red.log').open('w') as out: red=subprocess.run(command,cwd=root,env=env,stdout=out,stderr=subprocess.STDOUT).returncode
 assert red!=0 and 'four_hex_boundary_is_occupied' in (p/'mutation-red.log').read_text()
finally:
 target.write_bytes(original)
assert hashlib.sha256(target.read_bytes()).digest()==hashlib.sha256(original).digest()
with (p/'mutation-restored-green.log').open('w') as out: green=subprocess.run(command,cwd=root,env=env,stdout=out,stderr=subprocess.STDOUT).returncode
assert green==0
(p/'mutation-result.json').write_text(json.dumps({'mutation':'Host clearance <=4 to <=3; settlement clearance unchanged','red_exit':red,'restored_exit':green,'restored_sha256':hashlib.sha256(original).hexdigest()},indent=2)+'\n')
print('PASS: named boundary mutant RED, byte-exact restore, fresh GREEN')
