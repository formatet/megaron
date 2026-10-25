#!/usr/bin/env python3
"""Physical-source mutations; each arm uses a separate fresh DB. Always restore."""
from pathlib import Path
import os,subprocess,sys,json,hashlib
ROOT=Path(__file__).resolve().parents[1]
OUT=Path(sys.argv[1]).resolve();OUT.mkdir(parents=True,exist_ok=True)
arms=[
 ('dispatch-origin','server/internal/combat/route.go','r.OffsetMilli = dispatch % 1000','r.OffsetMilli = 0','TestShipReport43640c42_DeparturePhase'),
 ('reached-memory','server/internal/combat/place_reach.go','if v.route.Version == 2 {','if false {','TestShipReport43640c42_DeparturePhase'),
 ('future-memory','server/internal/combat/place_reach.go','entered[j] > v.last && entered[j] <= through','entered[j] > v.last','TestShipReport43640c42_DeparturePhase'),
 ('storm-cutoff','server/internal/combat/storm_timed.go','if through > end {','through = end\n if through > end {','TestTimedStormContactsDoNotReachFutureHexes'),
]
def digest():return {str(p.relative_to(ROOT)):hashlib.sha256(p.read_bytes()).hexdigest() for p in (ROOT/'server').rglob('*.go')}
before=digest();results=[]
for i,(name,rel,old,new,test) in enumerate(arms):
 p=ROOT/rel;original=p.read_bytes();s=original.decode();assert s.count(old)==1,(rel,old,s.count(old))
 try:
  p.write_text(s.replace(old,new))
  env={**os.environ,'GOTEST_PORT':str(56190+i)}
  r=subprocess.run(['tools/gotest.sh','./internal/combat/','-run',test,'-v'],cwd=ROOT,env=env,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True)
  (OUT/(name+'-red.log')).write_text(r.stdout)
  assert r.returncode!=0 and ('--- FAIL: '+test) in r.stdout,(name,r.returncode,r.stdout[-1200:])
  results.append({'name':name,'assertion_red':True,'test':test})
 finally:p.write_bytes(original)
 assert digest()==before,'source not restored'
r=subprocess.run(['tools/gotest.sh','./internal/combat/','-run','TestTimed|TestShipReport','-v'],cwd=ROOT,env={**os.environ,'GOTEST_PORT':'56195'},stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True)
(OUT/'restored-green.log').write_text(r.stdout);assert r.returncode==0,r.stdout[-1500:]
assert digest()==before
(OUT/'proof.json').write_text(json.dumps({'arms':results,'exact_restore':True,'source_sha256':before,'restored_green':True},indent=2))
print('PASS four assertion-red mutants, exact restores, final fresh green')
