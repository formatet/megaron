#!/usr/bin/env python3
"""Read-only geometry/rate audit. No claims about ownership, routes or self-sufficiency."""
from pathlib import Path
from collections import Counter
import hashlib
import json
import statistics

HERE=Path(__file__).resolve().parent
EXCLUDED={'coastal_sea','deep_sea','river','river_ford','mountain_limestone','mountain_red','semi_desert'}
ADDED={'scrub_maquis','hills','river_valley'}

def distance(a,b):return max(abs(a[0]-b[0]),abs(a[1]-b[1]),abs(a[0]-b[0]+a[1]-b[1]))

def ring(q,r,radius=2):
 return [(q+dq,r+dr) for dq in range(-radius,radius+1) for dr in range(-radius,radius+1) if 0<max(abs(dq),abs(dr),abs(dq+dr))<=radius]

def tally(tiles,centre,rates):
 terrain=Counter(tiles[c]['terrain'] for c in ring(*centre) if c in tiles)
 now=sum(n for t,n in terrain.items() if rates.get(t,0)>0)
 after=sum(n for t,n in terrain.items() if rates.get(t,0)>0 or t in ADDED)
 return dict(q=centre[0],r=centre[1],timber_hexes=now,proposed_timber_hexes=after,cedar_hexes=terrain['forest_cedar'],terrain=dict(sorted(terrain.items())))

def summary(rows):
 return dict(sites=len(rows),zero=sum(x['timber_hexes']==0 for x in rows),proposed_zero=sum(x['proposed_timber_hexes']==0 for x in rows),median=statistics.median(x['timber_hexes'] for x in rows),minimum=min(x['timber_hexes'] for x in rows),maximum=max(x['timber_hexes'] for x in rows),histogram=dict(sorted(Counter(x['timber_hexes'] for x in rows).items())))

def audit(name,raw,rates):
 tiles={(t['q'],t['r']):t for t in raw}
 candidates=sorted(c for c,t in tiles.items() if t['terrain'] not in EXCLUDED)
 rows=[tally(tiles,c,rates) for c in candidates]
 # A deterministic spatial sample, not the live SQL's ore/load-biased spawn algorithm.
 order=sorted(candidates,key=lambda c:hashlib.sha256(f'{name}:{c}'.encode()).digest())
 sample=[]
 for c in order:
  if all(distance(c,(r['q'],r['r']))>=5 for r in sample):sample.append(tally(tiles,c,rates))
 return dict(name=name,all_terrain_eligible=summary(rows),separated_sample=summary(sample),sample=sample),tiles

def main():
 live=json.loads((HERE/'live.json').read_text());assert live['read_only']=='on'
 # All current timber rules are ungated terrain rows; fail explicitly if that changes.
 timber=[r for r in live['rules'] if r['good_key']=='timber']
 assert all(r['terrain_type'] and r['building_type'] is None and r['requires_deposit'] is None and not r['requires_coastal'] for r in timber)
 rates={r['terrain_type']:r['rate_per_tick'] for r in timber}
 assert len(ring(0,0))==18 and (0,0) not in ring(0,0)
 assert tally({(0,0):{'terrain':'forest_olive_grove'}},(0,0),rates)['timber_hexes']==0,'centre must not produce'
 assert tally({(2,-1):{'terrain':'forest_olive_grove'}},(0,0),rates)['timber_hexes']==1,'radius two was lost'
 assert tally({(3,-1):{'terrain':'forest_olive_grove'}},(0,0),rates)['timber_hexes']==0,'radius three leaked in'
 assert tally({(1,0):{'terrain':'forest_cedar'}},(0,0),rates)['timber_hexes']==0,'cedar is not timber'
 worlds=[];cities=[]
 for w in live['worlds']:
  result,tiles=audit('live-'+w['id'],[t for t in live['tiles'] if t['world_id']==w['id']],rates)
  result['provenance']=w;worlds.append(result)
  for city in live['settlements']:
   if city['world_id']==w['id'] and city['state']=='active':cities.append({'name':city['name'],'population':city['population'],**tally(tiles,(city['q'],city['r']),rates)})
 for path in sorted(HERE.glob('fresh-*.json')):
  w=json.loads(path.read_text());result,_=audit(path.stem,w['tiles'],rates)
  result['provenance']={k:v for k,v in w.items() if k!='tiles'};worlds.append(result)
 land={t['terrain'] for t in live['tiles'] if t['terrain'] not in {'coastal_sea','deep_sea','river','river_ford'}}
 stone={r['terrain_type']:r['rate_per_tick'] for r in live['rules'] if r['good_key']=='stone' and r['building_type'] is None}
 result=dict(captured_at=live['captured_at'],migration=live['migration'],timber_rates=rates,stone_rates=stone,land_without_stone=sorted(land-set(stone)),worlds=worlds,active_cities=cities,city_summary=summary(cities),geometry_checks=['18 productive hexes; own centre excluded','radius2 included; radius3 excluded','cedar distinct from timber'])
 (HERE/'results.json').write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n')
 print('TIMBER',rates,'LAND_WITHOUT_STONE',result['land_without_stone'])
 for w in worlds:print(w['name'],w['all_terrain_eligible'],'SPATIAL_SAMPLE',w['separated_sample'])
 for c in cities:print(c['name'],c['timber_hexes'],'→',c['proposed_timber_hexes'],'cedar',c['cedar_hexes'])
 print('CITY_SUMMARY',result['city_summary'])

if __name__=='__main__':main()
