#!/usr/bin/env python3
"""Read-only final-rule impact, separate from earlier rule snapshots."""
from pathlib import Path
import json,sys,collections
root=Path(__file__).resolve().parents[3];p=Path(__file__).resolve().parent
sys.path.insert(0,str(root/'tools'))
from alfa_db import Db
s=Db(ssh='root@10.0.1.92').rows((p/'snapshot.sql').read_text())[0]
assert s['transaction_read_only']=='on'
(p/'final-live-snapshot.json').write_text(json.dumps(s,indent=2,ensure_ascii=False)+'\n')
rows=[]
for city in s['cities']:
 level=max([b['level'] for b in city['buildings'] if b['building_type']=='harbour'] or [0])
 tiles={(t['q'],t['r']):t for t in city['catchment']}
 groups=collections.defaultdict(list)
 for job in city['placements']:
  if job['target_kind']=='hex' and job['good_key']=='fish':groups[(job['hex_q'],job['hex_r'])].append(job['gubbe_ordinal'])
 for coord,ords in sorted(groups.items()):
  t=tiles.get(coord)
  if not t:raise SystemExit('placement outside saved catchment')
  water=t['terrain'];bay=water=='coastal_sea' and sum(n['terrain'] not in ('sea','coastal_sea','deep_sea') for n in t['neighbours'])>=4
  n=len(ords)
  if water=='deep_sea':cap=[0,1,2,4][min(3,level)];old=min(n,1)/6;new=min(n,cap)
  elif water=='coastal_sea':cap=2;old=min(n,6 if level else 2)*(1+.7*level);new=min(n,cap)*(2 if bay else 1)
  elif water in ('river','river_ford'):cap=1;old=new=min(n,1)*.5
  else:raise SystemExit('unexpected fish terrain '+water)
  rows.append({'city':city['name'],'q':coord[0],'r':coord[1],'terrain':water,'bay':bay,'harbour':level,'placed_before':n,'cap_after':cap,'freed_ordinals':sorted(ords)[cap:],'placed_fish_before':old,'placed_fish_after':new})
(p/'final-live-impact.json').write_text(json.dumps(rows,indent=2,ensure_ascii=False)+'\n')
lines=['# Final-rule live impact','',f"READ ONLY {s['observed_at']}; migration{s['migration']}. Calculation for placed fishing jobs; excludes hunger/stock/army/automatic food.",'','| City | Hex | Water | Bay | Harbour | Before workers | Cap | Freed ordinals | Fish before | Fish after |','|---|---|---|---|---:|---:|---:|---|---:|---:|']
for x in rows:lines.append(f"| {x['city']} | {x['q']},{x['r']} | {x['terrain']} | {x['bay']} | {x['harbour']} | {x['placed_before']} | {x['cap_after']} | {x['freed_ordinals']} | {x['placed_fish_before']:.6g} | {x['placed_fish_after']:.6g} |")
lines+=['',f"Placed fish total: {sum(x['placed_fish_before'] for x in rows):.6g} → {sum(x['placed_fish_after'] for x in rows):.6g}/tick; workers freed: {sum(len(x['freed_ordinals']) for x in rows)}.",'','This is a read-only manifest, not applied state. Re-run immediately before deploy; actual transition uses economy owner and prunes highest ordinals without auto-placement. No protection/backfill promised.']
(p/'final-live-impact.md').write_text('\n'.join(lines)+'\n')
print('\n'.join(lines[-4:]))
