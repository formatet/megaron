#!/usr/bin/env python3
"""Offline consistency gates, not game tests."""
import json,math
from pathlib import Path
p=Path(__file__).resolve().parent
s=json.loads((p/'live-snapshot.json').read_text());h=json.loads((p/'healthz.json').read_text())
assert s['transaction_read_only']=='on' and s['migration']=={'version':200,'dirty':False}
assert h['commit']=='d248b09' and h['migration']==200
food=['grain','fish','oil','wine'];cities={c['id']:c for c in s['cities']}
for name,pending in [('analysis.json',False),('analysis-pending.json',True)]:
 a=json.loads((p/name).read_text());assert len(a['cities'])==12
 for c in a['cities']:
  assert all(abs(c['rates'][g]-c['reconstructed'][g])<1e-8 for g in food+['silver'])
  assert c['idle']==c['population']//100-len(cities[c['id']]['placements'])
  assert c['unmet_actual']==0
  ev=c['upkeep_actual']
  if ev:
   assert abs(ev['payload']['food_total']-c['army_city_need']-c['ships_city_need'])<1e-8
   assert abs(ev['payload']['silver_gross']+ev['payload']['silver_unpaid']-c['silver_pay'])<1e-8
   assert ev['payload']['silver_circulated']==0 and ev['payload']['circulated_to']=={}
  prev=c['stocks'].copy();goods={g['good_key']:g for g in cities[c['id']]['goods']}
  for step in c['timeline']:
   production=0;spill=0
   for g in food:
    add=0 if pending and step['day']==1 else c['rates'][g]
    production+=add;spill+=max(0,prev[g]+add-goods[g]['cap'])
   value=lambda st:sum(st.get(g,0) for g in food)+3*st.get('livestock',0)
   used=lambda st:sum(st.get(g,0) for g in food)+3*st.get('livestock',0)
   loss=value(prev)+production-spill+3*step['born']-step['fed']-used(step['army_used'])-used(step['civil_used'])-sum(step['gift'].get(g,0) for g in food)-value(step['stocks'])
   assert loss>=-1e-7 and abs(loss/3-round(loss/3))<1e-7,(c['name'],step['day'],loss)
   assert all(v>=-1e-7 for v in step['stocks'].values())
   prev=step['stocks']
  expected=[step['day'] for step in c['timeline'] if step['civil_unmet']>0]
  assert c['first_civil_unmet_day_sim']==(min(expected) if expected else None)
 risk=[c['name'] for c in a['cities'] if c['first_civil_unmet_day_sim'] and c['first_civil_unmet_day_sim']<=30]
 assert risk==['Damon'],risk
 pha=next(c for c in a['cities'] if c['name']=='Phaistos')
 assert pha['upkeep_actual']['payload']['silver_unpaid']==.1 and pha['first_silver_short_day_sim']==1
print('PASS: read-only provenance; 60 reconstructed rates; 12 workforce counts; actual upkeep needs/pay; both forecast arms commodity mass balance; only Damon <=30; Phaistos actual unpaid.')
