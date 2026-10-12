#!/usr/bin/env python3
"""Physical RÖ1 mutations; each requires a named failing test and byte restoration."""
from pathlib import Path
import json, os, subprocess
ROOT=Path(__file__).resolve().parents[1]
OUT=ROOT/'docs/reviews/rorelsetabellen/mutations'
OUT.mkdir(parents=True,exist_ok=True)
movement='server/internal/province/movement.go'
pathfind='server/internal/province/pathfind.go'
cases=[]
for terrain,needle,replacement in [
 ('plains','case "plains", "river_valley", "river_delta":\n\t\treturn 1','case "plains", "river_valley", "river_delta":\n\t\treturn 0.75'),
 ('coastal_sea','return 1.0 / 3','return 0.4'),
 ('deep_sea','return 1.0 / 5','return 0.7'),
 ('river','return 0.5','return 0.6'),
 ('forest_olive_grove','return 1.5','return 1.25'),
 ('hills','case "hills", "forest_cedar":\n\t\treturn 2','case "hills", "forest_cedar":\n\t\treturn 1.25'),
 ('semi_desert','return 3','return 2')]:
 cases.append((terrain,movement,needle,replacement,'./internal/province','TestRO1TerrainCosts/'+terrain))
cases += [
 ('ford-land',movement,'return 2.5','return 1','./internal/province','TestFindPath_RiverFordLandCost'),
 ('ford-naval',movement,'return 1\n\t}\n\treturn TerrainMoveTicks(terrain)','return 2.5\n\t}\n\treturn TerrainMoveTicks(terrain)','./internal/province','TestFindPath_RiverFordNavalCost'),
 ('courier-river',pathfind,'return moveHoursFor(terrain, "naval")','return TerrainMoveTicks(terrain) / 2','./internal/province','TestRO1CourierLandAndRiver'),
 ('naval-heuristic',pathfind,'return TerrainMoveTicks("deep_sea")','return TerrainMoveTicks("coastal_sea")','./internal/province','TestFindPath_HeuristicAdmissibility_NavalDetourCheaperThanDirect'),
 ('caravan-factor','server/internal/province/trade_journey.go','ticks := int((total*3 + 1000) / 2000)','ticks := int((total*2 + 1000) / 2000)','./internal/province','TestRO1CaravanFactorAndFrozenJourney'),
]
env={k:v for k,v in os.environ.items() if k not in ('DATABASE_URL','TICK_SECONDS')}
results=[]
for name,filename,needle,replacement,pkg,test in cases:
 path=ROOT/filename;before=path.read_bytes();source=before.decode()
 assert needle in source,(name,'missing mutation anchor')
 try:
  path.write_text(source.replace(needle,replacement,1))
  red=subprocess.run(['go','test','-count=1',pkg,'-run','^'+test+'$'],cwd=ROOT/'server',env=env,text=True,capture_output=True)
  (OUT/(name+'-red.log')).write_text(red.stdout+red.stderr)
  assert red.returncode!=0 and '--- FAIL: '+test.split('/')[0] in red.stdout,(name,'no named red',red.stdout,red.stderr)
 finally:
  path.write_bytes(before)
 assert path.read_bytes()==before
 green=subprocess.run(['go','test','-count=1',pkg,'-run','^'+test+'$'],cwd=ROOT/'server',env=env,text=True,capture_output=True)
 (OUT/(name+'-restored.log')).write_text(green.stdout+green.stderr)
 assert green.returncode==0,(name,'restoration not green',green.stdout,green.stderr)
 results.append({'mutation':name,'test':test,'named_red':True,'byte_restored':True,'restored_green':True})
 print(name,'named red -> byte restored green',flush=True)
(OUT/'results.json').write_text(json.dumps(results,indent=2)+'\n')
