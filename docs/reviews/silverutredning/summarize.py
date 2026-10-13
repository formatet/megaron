#!/usr/bin/env python3
"""Capacity arithmetic and fairness; every measured point retained in maps-*.jsonl."""
import json,pathlib,statistics,sys
OUT=pathlib.Path(sys.argv[1]) if len(sys.argv)>1 else pathlib.Path(__file__).parent/'results'
def fairness(points):
 d=[p[2] for p in points];same=[p[3] for p in points if p[3] is not None]
 return {'n':len(d),'median':statistics.median(d),'max':max(d),
 'within4_pct':100*sum(v<=4 for v in d)/len(d),'within8_pct':100*sum(v<=8 for v in d)/len(d),
 'same_landmass_pct':100*len(same)/len(d),'same_landmass_within8_pct':100*sum(v<=8 for v in same)/len(d)}
maps={};summary={};baseline=None
for target in [4,5,10,20,40]:
 rows=[json.loads(s) for s in (OUT/f'maps-{target}.jsonl').read_text().splitlines()]
 assert len(rows)==20 and len({r['effective_seed'] for r in rows})==20
 if baseline is None:baseline=rows
 for slots in [5,10]:
  rates=[slots*(r['silver_hexes'].get('hills',0)+2.5*r['silver_hexes'].get('mountain_limestone',0)) for r in rows]
  l1=statistics.mean(rates);l3=l1*31/17
  data={'target_hexes':target,'slots_per_hex':slots,'mean_hexes':statistics.mean(len(r['deposits']) for r in rows),
   'hex_min':min(len(r['deposits']) for r in rows),'hex_max':max(len(r['deposits']) for r in rows),
   'workers_mean':slots*statistics.mean(len(r['deposits']) for r in rows),
   'l1_mean':l1,'l1_min':min(rates),'l1_max':max(rates),'l3_mean':l3,
   'l3_min':min(rates)*31/17,'l3_max':max(rates)*31/17,
   'per_wanax':{str(n):{'l1':l1/n,'l3':l3/n,'gross_spears_l1':l1/n,'net_spears_l1':l1/n/.3,'gross_spears_l3':l3/n,'net_spears_l3':l3/n/.3} for n in [10,50]},
   'gross_cohorts':{'l1':l1,'l3':l3},'garrison_net_cohorts':{'l1':l1/.3,'l3':l3/.3},
   'same_effective_seed_and_geography_as_baseline':sum(a['effective_seed']==b['effective_seed'] and a['non_silver_map_sha256']==b['non_silver_map_sha256'] for a,b in zip(rows,baseline)),
   'static_candidates':fairness([p for r in rows for p in r['candidates']]),
   'hosts':{str(n):fairness([p for r in rows for p in r['hosts'][:n]]) for n in [10,50]},
   'hosts_per_map_min':min(len(r['hosts']) for r in rows),'hosts_per_map_max':max(len(r['hosts']) for r in rows)}
  summary[f'{target}hex-{slots}slots']=data
maps={str(target):[{'requested_seed':r['requested_seed'],'effective_seed':r['effective_seed'],'hexes':len(r['deposits']),'hills':r['silver_hexes'].get('hills',0),'limestone':r['silver_hexes'].get('mountain_limestone',0),'l1_five_slots':5*(r['silver_hexes'].get('hills',0)+2.5*r['silver_hexes'].get('mountain_limestone',0)),'hosts':len(r['hosts'])} for r in [json.loads(s) for s in (OUT/f'maps-{target}.jsonl').read_text().splitlines()]] for target in [4,5,10,20,40]}
(OUT/'summary.json').write_text(json.dumps(summary,indent=2)+'\n')
(OUT/'seeds.json').write_text(json.dumps(maps,indent=2)+'\n')
for key,v in summary.items():
 print(key, 'hex',v['mean_hexes'],'L1/L3',round(v['l1_mean'],3),round(v['l3_mean'],3),'all',v['static_candidates'],'hosts10/50',v['hosts'])
