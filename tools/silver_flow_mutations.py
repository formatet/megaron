#!/usr/bin/env python3
"""Physically restore one old decision, require its named test to fail, restore bytes."""
from pathlib import Path
import subprocess, hashlib, json
ROOT=Path(__file__).resolve().parents[1]
OUT=ROOT/'docs/reviews/silverflodet/mutations'
OUT.mkdir(parents=True,exist_ok=True)
cases=[
 ('spearman_recruit','server/internal/province/training.go','"grain": 0.12}', '"grain": 0.12, "silver": 0.2}', './internal/province','TestSilverFlowRecruitCosts/spearman',False),
 ('galley_recruit','server/internal/province/training.go','"timber": 1.5}', '"timber": 1.5, "silver": 0.3}', './internal/province','TestSilverFlowRecruitCosts/galley',False),
 ('emporos_silver','server/internal/province/training.go','"cedar": 9.0}', '"cedar": 9.0, "silver": 0.2}', './internal/province','TestSilverFlowRecruitCosts/merchantman',False),
 ('emporos_wood','server/internal/province/training.go','"cedar": 9.0}', '"timber": 9.0}', './internal/province','TestSilverFlowRecruitCosts/merchantman',False),
 ('galley_pay','server/internal/economy/ship_ration.go','"galley": 0,','"galley": 1,','./internal/economy','TestSoldierWagesUseActualMenAndHomeHalf',False),
 ('emporos_pay','server/internal/economy/ship_ration.go','"merchantman": 0,','"merchantman": 1,','./internal/economy','TestSoldierWagesUseActualMenAndHomeHalf',False),
 ('chariot_pay','server/internal/economy/ship_ration.go','"war_chariot": 4,','"war_chariot": 2,','./internal/economy','TestSoldierWagesUseActualMenAndHomeHalf',False),
 ('spearman_dice','server/internal/combat/battle.go','diceCount = (active + 1) / 2','diceCount = active','./internal/combat','TestSilverFlowRoundDice/spearman',False),
 ('war_galley_dice','server/internal/combat/battle.go','case "war_chariot", "war_galley":','case "war_chariot":','./internal/combat','TestSilverFlowRoundDice/war_galley',False),
 ('host_purse','server/api/handlers/nomadic_host.go','nomadicHostStartingSilver = 120.0','nomadicHostStartingSilver = 480.0','./api/handlers','TestSeedNomadicHost_SilverAmountIs120',True),
 ('temple_tithe','server/internal/economy/trade.go',None,None,'./internal/economy','TestSilverFlowReligiousTradePaysFullSilver',True),
]
results=[]
for name,rel,old,new,pkg,test,db in cases:
 p=ROOT/rel;original=p.read_bytes();digest=hashlib.sha256(original).hexdigest()
 try:
  if old is None:
   p.write_bytes(subprocess.check_output(['git','-C',str(ROOT),'show','2e2cbcb3:'+rel]))
   (ROOT/'server/internal/economy/tithe.go').write_bytes(subprocess.check_output(['git','-C',str(ROOT),'show','2e2cbcb3:server/internal/economy/tithe.go']))
  else:
   s=original.decode();assert s.count(old)==1,(name,s.count(old));p.write_text(s.replace(old,new))
  cmd=[str(ROOT/'tools/gotest.sh'),pkg,'-run',test] if db else ['go','test',pkg,'-run',test,'-count=1']
  r=subprocess.run(cmd,cwd=ROOT if db else ROOT/'server',capture_output=True,text=True)
  (OUT/(name+'-red.log')).write_text(r.stdout+r.stderr)
  assert r.returncode!=0 and '--- FAIL: '+test.split('/')[0] in r.stdout+r.stderr, (name,'not a named assertion failure')
 finally:
  p.write_bytes(original)
  if old is None:(ROOT/'server/internal/economy/tithe.go').unlink()
 assert hashlib.sha256(p.read_bytes()).hexdigest()==digest
 results.append({'mutation':name,'test':test,'red':True,'restored_sha256':digest})
 print(name+': named RED -> exact restore',flush=True)
(OUT/'results.json').write_text(json.dumps(results,indent=2)+'\n')
