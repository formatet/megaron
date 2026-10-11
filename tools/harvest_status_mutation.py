#!/usr/bin/env python3
"""Physical harvest visibility mutations; restore exact bytes after each probe."""
from pathlib import Path
import subprocess,json,hashlib
ROOT=Path(__file__).resolve().parents[1];OUT=ROOT/'docs/reviews/skordegava-syns';results=[]
cases=[
 ('api-expiry','server/api/handlers/province.go','*until <= currentTick','*until < currentTick',['tools/gotest.sh','./api/handlers','-run','TestProvinceHarvestBlessingStatus'],'ended harvest blessing leaked'),
 ('api-bonus','server/api/handlers/province.go','"yield_bonus":     economy.HarvestBlessingYieldBonus','"yield_bonus":     0.0',['tools/gotest.sh','./api/handlers','-run','TestProvinceHarvestBlessingStatus'],'harvest status contradicts'),
 ('web-row','web/static/js/megaron/ui/drawers/city_production.js','const b = pd?.harvest_blessing;','const b = null;',['node','--test','web/static/js/megaron/ui/drawers/city_production.test.mjs'],'active harvest missing'),
 ('keryx-status','server/cmd/keryx/cmd_status.go','if line := harvestBlessingLine(sett); line != "" {','if line := ""; line != "" {',['go','test','./cmd/keryx','-run','TestHarvestBlessingBothCityCommands','-count=1'],'status missing authoritative harvest row'),
 ('keryx-city','server/cmd/keryx/cmd_city.go','fmt.Println(fs.HarvestBlessing)','fmt.Print("")',['go','test','./cmd/keryx','-run','TestHarvestBlessingBothCityCommands','-count=1'],'city missing authoritative harvest row'),
]
for name,file,old,new,cmd,red in cases:
 p=ROOT/file;original=p.read_bytes();assert old in original.decode(),(name,old)
 try:
  p.write_text(original.decode().replace(old,new,1))
  r=subprocess.run(cmd,cwd=ROOT/'server' if cmd[0]=='go' else ROOT,capture_output=True,text=True)
  log=r.stdout+r.stderr;(OUT/(name+'-mutation.log')).write_text(log)
  assert r.returncode!=0 and red in log,(name,log)
 finally:p.write_bytes(original)
 assert p.read_bytes()==original
 results.append({'mutation':name,'red':red,'restored_sha256':hashlib.sha256(original).hexdigest()});print(name,'named red, byte restored',flush=True)
(OUT/'mutations.json').write_text(json.dumps(results,indent=2)+'\n')
