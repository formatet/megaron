#!/usr/bin/env python3
from pathlib import Path
import subprocess,hashlib,json
root=Path(__file__).resolve().parents[3];out=Path(__file__).resolve().parent
cases=[('debt','server/internal/kharis/demolition.go','TempleDemolitionPenaltyPerLevel = 10.0','TempleDemolitionPenaltyPerLevel = 0.0',['./api/handlers','-run','^TestTempleDemolitionDebtFloorReplay$']),('ceiling','server/internal/kharis/tick.go','if current > cap {','if false {',['./internal/kharis','-run','^TestLoweredCeilingDriftsWithoutCliffAndForecastMatches$'])]
hashes={}
for name,path,before,after,args in cases:
 p=root/path;original=p.read_bytes();assert before in original.decode();hashes[path]=hashlib.sha256(original).hexdigest()
 try:
  p.write_text(original.decode().replace(before,after,1))
  with (out/f'mutation-{name}-red.log').open('w') as log:r=subprocess.run(['bash','tools/gotest.sh',*args],cwd=root,stdout=log,stderr=subprocess.STDOUT)
  assert r.returncode!=0 and '--- FAIL: Test' in (out/f'mutation-{name}-red.log').read_text(),name
 finally:p.write_bytes(original)
 assert hashlib.sha256(p.read_bytes()).hexdigest()==hashes[path]
with (out/'mutation-restored-green.log').open('w') as log:r=subprocess.run(['bash','tools/gotest.sh','./api/handlers','./internal/kharis','./internal/combat','-run','TestTempleDemolition|TestLoweredCeiling|TestHostileSack'],cwd=root,stdout=log,stderr=subprocess.STDOUT)
assert r.returncode==0
(out/'mutation.json').write_text(json.dumps({'physical_red_cases':[c[0] for c in cases],'byte_exact_restore':hashes,'restored_green':True},indent=2)+'\n')
