#!/usr/bin/env python3
"""Physical, assertion-red mutations in a separate checkout; exact byte restoration."""
from pathlib import Path
import subprocess,json,sys
r=Path(sys.argv[1]).resolve();out=Path(sys.argv[2]).resolve();out.mkdir(parents=True,exist_ok=True)
cases=[
 ('server/internal/kharis/standing.go','return blessProbability','return 0','./internal/kharis/','TestStandingCurrentConsequences'),
 ('server/internal/kharis/tick.go','blessProbability = 0.15','blessProbability = 0.16','./internal/kharis/','TestStandingCurrentConsequences'),
 ('server/api/handlers/province.go','kharisStanding = &standing','kharisStanding = nil; _ = standing','./api/handlers/','TestProvinceStandingMatchesTickOwner'),
 ('server/api/handlers/world_me.go','cult.DescribeStanding(kharis.Amount, kharis.MaxTempleLevel > 0, wrath.ProtectionRemainingDays)','cult.Standing{}; _ = wrath','./api/handlers/','TestWorldMe_OwnWorldIsolation'),
 ('server/internal/kharis/demolition.go','DescribeStanding(d.KharisAfter, afterLevel > 0, wrath.ProtectionRemainingDays)','Standing{}; _ = wrath','./api/handlers/','TestTempleDemolitionDebtFloorReplay'),
 ('server/internal/kharis/transition_notices.go','DescribeStanding(amount, w.templeCities > 0, wrath.ProtectionRemainingDays)','Standing{}; _ = wrath','./internal/kharis/','TestKharisBandNoticeBoundariesAndExternalChanges'),
]
proof=[]
for i,(file,old,new,pkg,test) in enumerate(cases):
 p=r/file;before=p.read_bytes();assert old in before.decode(),file
 try:
  p.write_text(before.decode().replace(old,new,1))
  res=subprocess.run([str(r/'tools/gotest.sh'),pkg,'-run','^'+test+'$'],cwd=r,capture_output=True,text=True)
  log=res.stdout+res.stderr;(out/f'{i}-red.log').write_text(log)
  assert res.returncode and '--- FAIL: '+test in log and '[build failed]' not in log,(file,log)
  proof.append({'file':file,'replace':old,'with':new,'test':test,'assertion_red':True})
 finally:p.write_bytes(before)
 assert p.read_bytes()==before
 print('RED + exact restore',file,flush=True)
res=subprocess.run([str(r/'tools/gotest.sh'),'./api/handlers/','./internal/kharis/','-run','TestProvinceStanding|TestStandingCurrent|TestWorldMe|TestTempleDemolitionDebt|TestKharisBandNotice'],cwd=r,capture_output=True,text=True)
(out/'restored-green.log').write_text(res.stdout+res.stderr);assert res.returncode==0,res.stdout+res.stderr
(out/'proof.json').write_text(json.dumps({'mutations':proof,'exact_restore':True,'fresh_restored_green':True},indent=2))
print('PASS mutations and restored fresh green',flush=True)
