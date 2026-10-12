#!/usr/bin/env python3
"""STOPP 1 arithmetic from READ ONLY snapshot and real mapgen output."""
import json, math, re
from pathlib import Path
D=Path(__file__).resolve().parent
rows=[json.loads(l) for l in (D/'dev-snapshot.jsonl').read_text().splitlines() if l.startswith('{')]
goods={r['row']['key']:r['row'] for r in rows if r['table']=='goods'}
rules=[r['row'] for r in rows if r['table']=='production_rules']
def rate(g,t=None,b=None):
    found=[r['rate_per_tick'] for r in rules if r['good_key']==g and r['terrain_type']==t and r['building_type']==b]
    assert len(found)==1,(g,t,b,found)
    return found[0]
standard={'grain':'plains','fish':'coastal_sea','livestock':'plains','timber':'forest_olive_grove','cedar':'forest_cedar','copper':'hills','tin':'mountain_limestone','stone':'plains','oil':'forest_olive_grove','wine':'hills'}
values={g:1/rate(g,t) for g,t in standard.items()}
inputs={r['row']['good_key']:r['row']['quantity'] for r in rows if r['table']=='recipe_ingredients'}
values['bronze']=1/rate('bronze',None,'foundry')+sum(q*values[g] for g,q in inputs.items())
values['silver']=1 # Timothy anchor, proposed hills L1; not the live DB rate
values['horses']=1/rate('horses',None,'stable') # dormant rule only
values['pottery']=1/rate('pottery',None,'market') # dormant rule only
values['purple']=None # no production rule
values['cult']=None # labor, not a commodity
assert set(values)==set(goods) and len(goods)==16
assert math.isclose(values['grain'],1), 'STANDARD_GRAIN_WORKDAY_REFERENCE'
assert math.isclose(values['bronze'],11), 'BRONZE_FULL_INPUT_CHAIN'
assert math.isclose(values['stone'],2), 'FIELDSTONE_WORKDAY_REFERENCE'
assert inputs=={'copper':9,'tin':1}, 'BRONZE_INPUT_RECIPE'
# P6 raw extraction already yields the output good; refining adds boost only.
p6={}
for g,t,b in [('oil','forest_olive_grove','olive_press'),('wine','hills','winery')]:
    raw,boost,capacity=rate(g,t),rate(g,t,b),rate(g,None,b)
    p6[g]={'raw':raw,'boost':boost,'refiner':capacity,'workers_per_extractor':1+boost/capacity,'output_per_extractor':raw+boost,'labor_per_unit':(1+boost/capacity)/(raw+boost),'one_plus_one_labor_per_unit':2/(raw+min(boost,capacity))}
assert math.isclose(p6['oil']['labor_per_unit'],9/16)
assert math.isclose(p6['wine']['labor_per_unit'],3/7)
maps=[json.loads(l) for l in (D/'mapgen.jsonl').read_text().splitlines()]
sample=[m for m in maps if 1009<=m['requested_seed']<=20180]
assert len(sample)==20 and len({m['effective_seed'] for m in sample})==20
factor=(1+.7*3)/(1+.7*1)
def output(h,m):return 5*(h+2.5*m)
for m in maps:
    c=m['silver_hexes'];assert set(c)<= {'hills','mountain_limestone'}, 'SILVER_DEPOSIT_TERRAINS'
    assert sum(c.values())==4, 'MEASURED_SILVER_HEX_CAPACITY'
    m['workers']=5*sum(c.values());m['silver_L1']=output(c.get('hills',0),c.get('mountain_limestone',0));m['silver_L3']=m['silver_L1']*factor
S=sum(m['silver_L1'] for m in sample)/20
assert math.isclose(S,27.875)
repo=D.parents[2]
upkeep_source=(repo/'server/internal/combat/upkeep.go').read_text()
unit_source=(repo/'server/internal/province/training.go').read_text()
upkeep={typ:{'grain':float(g),'silver':float(s)} for typ,g,s in re.findall(r'"(\w+)":\s*\{Grain:\s*([0-9.]+), Silver:\s*([0-9.]+)\}',upkeep_source)}
sold={typ:v['silver'] for typ,v in upkeep.items()}
assert sold=={'spearman':1,'elite_infantry':2,'war_chariot':3,'galley':1.5,'war_galley':2.5,'merchantman':1}, 'LOCKED_UNIT_PAYROLL_ANCHORS'
unit_costs={}
crew={'galley':20,'war_galley':50,'merchantman':10}
for typ,body in re.findall(r'"(\w+)":\s*\{Costs: map\[string\]float64\{([^}]+)\}',unit_source):
    n=crew.get(typ,100)
    unit_costs[typ]={g:float(q)*n for g,q in re.findall(r'"(\w+)":\s*([0-9.]+)',body)}
assert unit_costs['spearman']=={'grain':12,'silver':20}, 'SPEARMAN_RECRUIT_CATALOGUE'
assert unit_costs['war_galley']=={'cedar':100,'silver':30}, 'SHIP_COST_USES_CREW_NOT_ONE'
assert set(unit_costs)==set(sold), 'ALL_SIX_RECRUIT_TYPES'

scenarios={'dev':20,'new_64x64_mean':S,'sample_low':min(m['silver_L1'] for m in sample),'sample_high':max(m['silver_L1'] for m in sample),'theoretical_4_limestone':50}
army={}
for name,s in scenarios.items():
    army[name]={}
    for level,prod in [(1,s),(3,s*factor)]:
        army[name][str(level)]={'silver_per_tick':prod,'per_10_wanaxes':prod/10,'gross_army_equivalents':{u:prod/pay for u,pay in sold.items()},'whole_units':{u:math.floor(prod/pay+1e-9) for u,pay in sold.items()},'same_payer_garrison_net_equivalents_default_share_0_7':{u:prod/(pay*.3) for u,pay in sold.items()}}
result={'unit_upkeep':upkeep,'unit_recruit_costs':unit_costs,'base_commit':'acf5c96f','goods':{g:{'current_base_value':goods[g]['base_value'],'proposed_labor_value':v} for g,v in values.items()},'P6':p6,'mapgen':maps,'sample_mean_L1':S,'sample_mean_L3':S*factor,'L3_to_L1':factor,'armies':army}
(D/'calculations.json').write_text(json.dumps(result,indent=2)+'\n')
print('PASS: 16 goods; bronze chain 11; fieldstone 2; P6 separate labor; 20 distinct new maps; 4 silver hexes/map; L1 mean 27.875; L3 ratio 31/17; all six payroll types.')
