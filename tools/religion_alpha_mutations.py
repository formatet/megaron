#!/usr/bin/env python3
"""Physical checkpoint-A mutations, named failures, byte restore, fresh DB proof."""
from pathlib import Path
import os, subprocess, json
ROOT=Path(__file__).resolve().parents[1];OUT=ROOT/'docs/reviews/religion-alfa/mutations-a';OUT.mkdir(parents=True,exist_ok=True)
cases=[
 ('vision-radius','server/api/handlers/oracle_memory.go','R: c.p.R}, 2)','R: c.p.R}, 1)','./api/handlers','TestAlphaVisionOnlyNewMemoryRadiusTwo'),
 ('walls-consumer','server/internal/combat/battle.go','if participants[i].defenseBoost {','if false && participants[i].defenseBoost {','./internal/combat','TestAlphaWallsRealDefenderStrengthAndOwnerExpiry'),
 ('walls-owner','server/internal/combat/battle.go','AND s.garrison_defense_owner_id=u.owner_id','AND true','./internal/combat','TestAlphaWallsRealDefenderStrengthAndOwnerExpiry'),
 ('green-span','server/internal/economy/harvest_blessing.go','HarvestBlessingTicks      = 25','HarvestBlessingTicks      = 24','./api/handlers','TestAlphaGreenShootsCannotRenewActiveHarvest'),
 ('green-renew','server/api/handlers/settlement.go','if active > 0 {','if active > 0 && spec.EffectType != religion.EffectHarvestBlessing {','./api/handlers','TestAlphaGreenShootsCannotRenewActiveHarvest'),
 ('silent-wait','server/api/handlers/settlement.go','if success && spec.GodID != "" {','if spec.GodID != "" {','./api/handlers','TestAlphaPrayerSharedGodCooldownAndSilentAnswer'),
 ('shared-god','server/internal/religion/prayers.go','GodID: "potnia", EffectType: EffectGarrisonDefense','GodID: "potnia_walls", EffectType: EffectGarrisonDefense','./api/handlers','TestAlphaPrayerSharedGodCooldownAndSilentAnswer'),
 ('silver-floor','server/internal/religion/offering.go','SilverAltarValueFloor = 100.0','SilverAltarValueFloor = 1.0','./internal/religion','TestAlphaCatalogueAndSilverFloor'),
 ('memory-activity','server/api/handlers/world.go','m.Besieged = false','m.Besieged = m.Besieged','./api/handlers','TestAlphaVisionMemoryAPIHidesActivity'),
]
results=[]
for name,file,needle,replacement,pkg,test in cases:
 p=ROOT/file;original=p.read_bytes();source=original.decode();assert needle in source,(name,'missing mutation anchor')
 try:
  p.write_text(source.replace(needle,replacement,1))
  if pkg=='./internal/religion':cmd=['go','-C',str(ROOT/'server'),'test','-count=1',pkg,'-run','^'+test+'$']
  else:cmd=[str(ROOT/'tools/gotest.sh'),pkg,'-run','^'+test+'$']
  env={k:v for k,v in os.environ.items() if k not in ('DATABASE_URL','TICK_SECONDS')}
  r=subprocess.run(cmd,cwd=ROOT,env=env,text=True,capture_output=True)
  (OUT/(name+'-red.log')).write_text(r.stdout+r.stderr)
  assert r.returncode!=0 and '--- FAIL: '+test in r.stdout,(name,'missing named failure',r.stdout,r.stderr)
 finally:p.write_bytes(original)
 assert p.read_bytes()==original
 results.append({'mutation':name,'test':test,'named_red':True,'byte_restored':True})
 print(name,'named red; byte restored',flush=True)
r=subprocess.run([str(ROOT/'tools/gotest.sh'),'./api/handlers','./internal/combat','./internal/religion','-run','^TestAlpha'],cwd=ROOT,text=True,capture_output=True)
(OUT/'restored-fresh.log').write_text(r.stdout+r.stderr);assert r.returncode==0,(r.stdout,r.stderr)
for item in results:item['restored_fresh_green']=True
(OUT/'results.json').write_text(json.dumps(results,indent=2)+'\n')
