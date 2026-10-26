#!/usr/bin/env python3
"""Physical mutations in an isolated checkout; require named assertion failures."""
from pathlib import Path
import json,os,subprocess,sys
root=Path(sys.argv[1]).resolve();out=Path(sys.argv[2]).resolve();out.mkdir(parents=True,exist_ok=True)
cases=[
 ('bare-prefix','server/internal/combat/place_reach.go','if v.displayID == v.id {','if false {','./internal/combat/','TestRouteHexRetainsFullSightBetweenPolls','passed middle hex lost'),
 ('no-departure-eye','server/internal/combat/place_reach.go','first = 0 // departure','first = 1 // departure','./internal/combat/','TestRouteHexSightStartsAtExactBoundary','start=false'),
 ('ship-as-land','server/internal/combat/place_reach.go','kind = province.EyeShip','kind = province.EyeLandUnit','./internal/combat/','TestRouteHexShipRetainsCoastButNotInland','ship sight:'),
 ('backfill-future','server/cmd/backfill-route-sight/main.go','if int64(at) <= last {','if int64(at) <= last || true {','./cmd/backfill-route-sight','TestBackfillOnlyWitnessedPrefixAndDryRunRollback','prefix missing or future revealed'),
]
proof=[]
for i,(name,file,old,new,pkg,test,assertion) in enumerate(cases):
 p=root/file;before=p.read_bytes();s=before.decode();assert old in s,(name,'missing mutation target')
 try:
  p.write_text(s.replace(old,new,1))
  env={**os.environ,'GOTEST_PORT':str(56750+i)}
  result=subprocess.run([str(root/'tools/gotest.sh'),pkg,'-run','^'+test+'$'],cwd=root,env=env,capture_output=True,text=True)
  log=result.stdout+result.stderr;(out/(name+'.log')).write_text(log)
  assert result.returncode!=0 and '--- FAIL: '+test in log and assertion in log,(name,log)
  proof.append({'name':name,'file':file,'replace':old,'with':new,'test':test,'assertion':assertion,'red':True})
 finally:
  p.write_bytes(before);assert p.read_bytes()==before
(out/'proof.json').write_text(json.dumps(proof,indent=2)+'\n')
print('PASS',len(proof),'assertion-red mutations, exact restoration')
