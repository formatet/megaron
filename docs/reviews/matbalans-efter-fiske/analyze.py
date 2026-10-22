#!/usr/bin/env python3
"""Offline, frozen-snapshot balance. No DB connection or game mutation."""
import json,math,collections,os
from pathlib import Path
P=Path(__file__).resolve().parent
S=json.loads((P/'live-snapshot.json').read_text()); OLD=json.loads((P.parent/'fiske-hamn-vik/final-live-snapshot.json').read_text())
FOOD=['grain','fish','oil','wine']; GOODS=FOOD+['silver']; ACTIVE={'garrison','marching','positioned','embarked','repairing','freighting','fitting_out'}
C={c['id']:c for c in S['cities']}; W={w['id']:w for w in S['worlds']}; U={u['id']:u for u in S['units']}
DEPS={'copper':'copper_deposit','tin':'tin_deposit','silver':'silver_deposit','incense':'incense_deposit','cedar':'cedar_deposit'}
def production(c, old=False):
 # Independent transcription of hex_rules/placement_yield/recompute for food+silver only.
 tick=W[c['world_id']]['current_tick']; buildings=c['buildings']; tiles={(t['q'],t['r']):t for t in c['catchment']}
 counts=collections.Counter((p['target_kind'],p.get('hex_q'),p.get('hex_r'),p.get('building_type'),p['good_key']) for p in c['placements'])
 def lv(bt,xy=None):return max([b['level'] for b in buildings if b['building_type']==bt and (bt not in ['farm','mine','lumbermill','stonequarry'] or (b['hex_q'],b['hex_r'])==xy)]+[0])
 def option(t,g):
  xy=t['q'],t['r'];ter=t['terrain']; base=boost=0
  for r in S['rules']:
   if r['good_key']!=g or r['terrain_type'] not in (None,ter):continue
   bt=r['building_type'];dep=r['requires_deposit']
   if r['terrain_type'] is None and bt:continue
   if ter in ['deep_sea','coastal_sea','river','river_ford'] and r['terrain_type']!=ter:continue
   if r['requires_coastal'] and not t['coastal']:continue
   if dep and not t.get(DEPS[dep]):continue
   if bt and not lv(bt,xy):continue
   if bt and bt=={'oil':'olive_press','wine':'winery'}.get(g):boost+=r['rate_per_tick']
   else:base+=r['rate_per_tick']
  cap=2;mult=1
  if g=='grain':
   cap={'plains':4,'hills':1,'river_valley':2,'river_delta':3}.get(ter,2)
   level=lv('farm',xy)
   if ter in ['plains','hills','river_valley','river_delta'] and level:cap+=4;mult=1+.7*level
  elif g=='fish':
   if ter=='deep_sea':cap=[0,1,2,4][min(3,lv('harbour'))];base=(1/6 if old else base)
   elif ter=='coastal_sea':cap=2;mult=2 if t.get('land_neighbors',sum(n['terrain'] not in ['sea','coastal_sea','deep_sea'] for n in t.get('neighbours',[])))>=4 else 1
   elif ter in ['river','river_ford']:cap=1
   if old and ter=='coastal_sea':cap=6 if lv('harbour') else 2;mult=1+.7*lv('harbour')
   if old and ter=='deep_sea':cap=1 if lv('harbour') else 0
  elif g=='silver':
   cap=1
   if lv('mine',xy):cap+=4;mult=1+.7*lv('mine',xy)
  return base,boost,cap,mult
 rates=collections.defaultdict(float);detail=[];boosts=collections.defaultdict(float);best=(0,'')
 for xy,t in tiles.items():
  for g in GOODS:
   base,boost,cap,mult=option(t,g)
   n=counts['hex',*xy,None,g];used=min(n,cap)
   qty=base*mult*used
   if n:detail.append({'q':xy[0],'r':xy[1],'terrain':t['terrain'],'good':g,'placed':n,'served':used,'r0':base,'mult':mult,'rate':qty,'farm':lv('farm',xy)})
   rates[g]+=qty;boosts[g]+=boost*mult*used
   if g in ['grain','fish'] and cap>0 and (base*mult>best[0] or base*mult==best[0] and g=='grain'):best=(base*mult,g)
 for g,bt in [('oil','olive_press'),('wine','winery')]:
  slots={0:0,1:1,2:2,3:4}[min(3,lv(bt))]
  n=counts['building',None,None,bt,g]
  r=sum(r['rate_per_tick'] for r in S['rules'] if r['terrain_type'] is None and r['building_type']==bt and r['good_key']==g)
  rates[g]+=min(boosts[g],r*slots*min(n,1))
 auto={'grain':.5,'fish':0}
 auto[best[1] or 'grain']+=(c['population']%100)/100*best[0]
 for g,n in auto.items():rates[g]+=n
 if c.get('harvest_blessing_until_tick') and c['harvest_blessing_until_tick']>tick:
  for g in ['grain','oil','wine']:rates[g]*=1.25
 return dict(rates),detail,auto

def upkeep(u):
 if u['status'] not in ACTIVE:return 0,0
 if any(f['owner_id']==u['owner_id'] and f['world_id']==u['world_id'] for f in S['founders']):return 0,0
 field=u['status'] in ['marching','positioned'] or u['category']=='naval' and u['status']=='freighting' or u['category']=='land' and u['status']=='embarked'
 n=u['crew'] if u['category']=='naval' else u['size']
 grain={'spearman':.5,'elite_infantry':.6,'war_chariot':.8,'galley':.5,'war_galley':.5,'merchantman':.5}.get(u['type'],0)*n/100*(2 if field else 1)
 pay={'spearman':0,'elite_infantry':1.5,'war_chariot':2,'galley':1,'war_galley':1,'merchantman':1}.get(u['type'],0)*n/100*(1 if field else .5)
 return grain,pay

def food_source(u):
 if u['category']=='naval':
  if u['status'] in ['garrison','repairing','fitting_out']:return u.get('support_settlement_id'),'port'
  if u['status']=='freighting':
   ts=sorted([t for t in S['transports'] if t['ship_unit_id']==u['id']],key=lambda t:(t['created_at'],t['id']))
   if not any(t['status']=='in_transit' for t in ts):
    sid=ts[-1]['dest_id'] if ts and ts[-1]['status']=='delivered' else u['settlement_id'] if not ts else None
    if sid in C and C[sid]['owner_id']==u['owner_id']:return sid,'freight_port'
  return None,'provisions'
 if u['status']=='embarked':return None,'carrier_provisions'
 return u.get('support_settlement_id'),'city'

def meal(stock,need,emergency=False,herdcap=math.inf):
 used=collections.defaultdict(float);unmet=need
 over=math.floor(max(0,stock['livestock']-herdcap)) if math.isfinite(herdcap) else 0
 eat=min(math.ceil(unmet/3),over);used['livestock']=eat;unmet=max(0,unmet-3*eat)
 total=sum(max(0,stock[g]) for g in FOOD);take=min(unmet,total)
 if total:
  for g in FOOD:used[g]=max(0,stock[g])*take/total
 unmet-=take
 if emergency and unmet>1e-9:
  animals=min(math.floor(stock['livestock']-used['livestock']),math.ceil(unmet/3));used['livestock']+=animals;unmet=max(0,unmet-3*animals)
 if unmet<1e-9:unmet=0
 # Army ration: all-or-nothing, no partial debit.
 if not emergency and unmet:return {},unmet
 for g,n in used.items():stock[g]-=n
 if math.isfinite(herdcap):stock['livestock']=min(stock['livestock'],herdcap)
 return dict(used),unmet

results=[];unitrows=[]
for u in S['units']:
 food,pay=upkeep(u);sid,source=food_source(u)
 unitrows.append({'id':u['id'],'type':u['type'],'status':u['status'],'people':u['crew'] if u['category']=='naval' else u['size'],'payer':u.get('support_settlement_id'),'food_city':sid,'food_source':source,'food_need':food,'silver_need':pay,'provisions':u['provisions'],'provision_days':u['provisions']/food if food else None,'unpaid_periods':u['unpaid_periods']})
for c in S['cities']:
 goods={g['good_key']:g for g in c['goods']};rate={g:goods.get(g,{}).get('rate',0) for g in GOODS};stock={g:goods.get(g,{}).get('settled_stock',0) for g in GOODS+['livestock','timber']}
 calc,details,auto=production(c);diff={g:calc.get(g,0)-rate[g] for g in GOODS}
 assert max(map(abs,diff.values()))<1e-8,(c['name'],diff)
 army=sum(u['food_need'] for u in unitrows if u['food_city']==c['id'] and U[u['id']]['category']=='land')
 ships=sum(u['food_need'] for u in unitrows if u['food_city']==c['id'] and U[u['id']]['category']=='naval')
 pay=sum(u['silver_need'] for u in unitrows if u['payer']==c['id'])
 civ=c['population']*.005;demand=civ+army+ships;food=sum(rate[g] for g in FOOD)
 ev=[e for e in S['recent_events'] if e['event_type']=='UpkeepSettled' and e['stream_id']==c['id']];last=max(ev,key=lambda e:(e['world_tick'],e['id'])) if ev else None
 old=next((o for o in OLD['cities'] if o['id']==c['id']),None)
 oldrates={g['good_key']:g['rate'] for g in old['food_goods']} if old else {}
 totalstock=sum(stock[g] for g in FOOD)
 # Per-food proportional next meal and frozen-share runway are diagnostics only.
 share={g:stock[g]/totalstock if totalstock else 0 for g in FOOD}
 next_use={g:demand*share[g] for g in FOOD}
 runway={g:(stock[g]/(next_use[g]-rate[g]) if next_use[g]>rate[g]+1e-9 else None) for g in FOOD}
 zero={g:(0 if stock[g]<=1e-9 else None) for g in FOOD}
 st=stock.copy();timeline=[];first=None;silverfail=None;pen=max([b['level'] for b in c['buildings'] if b['building_type']=='pen']+[0]);cap=50+100*pen
 temple=any(b['building_type']=='temple' for b in c['buildings']);vals={v['good_key']:v['divine_value'] for v in S['valuations'] if v['world_id']==c['world_id']}
 for day in range(1,31):
  tick=W[c['world_id']]['current_tick']+day-(1 if os.environ.get('PENDING_MEAL') else 0)
  for g in GOODS:st[g]=min(goods.get(g,{}).get('cap',math.inf),max(0,st[g]+(0 if day==1 and os.environ.get('PENDING_MEAL') else rate[g])))
  if st['silver']+1e-9<pay and silverfail is None:silverfail=day
  # payroll whole units, in fixed UUID order; net need is reported separately.
  for u in sorted([u for u in unitrows if u['payer']==c['id']],key=lambda u:u['id']):
   if st['silver']+1e-9>=u['silver_need']:st['silver']-=u['silver_need']
  armymeal,armyunmet=meal(st,army+ships)
  born=fed=0
  if tick%5==0:
   calves=math.floor(max(0,st['livestock'])/10);fed=min(calves,math.floor(max(0,st['grain'])));born=calves if tick%20==0 else fed;st['grain']-=fed;st['livestock']+=born
  used,unmet=meal(st,civ,True,cap)
  gift={}
  if temple:
   due=.25*vals.get('grain',0);plan={}
   if st['grain']>=.25:plan={'grain':.25};due=0
   elif due>0:
    for g in ['grain','fish','timber','silver']:
     value=vals.get(g,0)
     if value<=0:continue
     take=min(st[g],due/value);plan[g]=take;due-=take*value
     if due<1e-9:break
   if due<1e-9:
    for g,n in plan.items():st[g]-=n
    gift=plan
  if unmet>0 and first is None:first=day
  for g in FOOD:
   if st[g]<=1e-9 and zero[g] is None:zero[g]=day
  timeline.append({'day':day,'tick':tick,'stocks':st.copy(),'civil_used':used,'civil_unmet':unmet,'army_used':armymeal,'army_unmet':armyunmet,'born':born,'fed':fed,'gift':gift})
  if first is not None and day>=30:break
 result={'id':c['id'],'name':c['name'],'population':c['population'],'rates':rate,'reconstructed':calc,'placement_detail':details,'automatic':auto,'stocks':stock,'civil_need':civ,'army_city_need':army,'ships_city_need':ships,'food_need':demand,'food_net':food-demand,'silver_pay':pay,'silver_net':rate['silver']-pay,'idle':c['population']//100-len(c['placements']),'unmet_actual':c['food_unmet_amount'],'actual_food_tick':c['food_consumed_tick'],'actual_meal':c['food_consumed'],'upkeep_actual':last,'food_runway_frozen_shares':runway,'food_next_use':next_use,'food_zero_day_sim':zero,'first_civil_unmet_day_sim':first,'first_silver_short_day_sim':silverfail,'old_rates':oldrates,'old_population':old['population'] if old else None,'old_placements':len(old['placements']) if old else None,'timeline':timeline}
 results.append(result)
(P/('analysis-pending.json' if os.environ.get('PENDING_MEAL') else 'analysis.json')).write_text(json.dumps({'snapshot_at':S['observed_at'],'old_at':OLD['observed_at'],'units':unitrows,'cities':results},indent=2)+'\n')
for r in results:print(r['name'],'net',round(r['food_net'],3),'stock',round(sum(r['stocks'][g] for g in FOOD),2),'idle',r['idle'],'hunger',r['first_civil_unmet_day_sim'],'silver',round(r['stocks']['silver'],2),'pay',r['silver_pay'],'silverfail',r['first_silver_short_day_sim'])
