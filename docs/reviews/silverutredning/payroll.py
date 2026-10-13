#!/usr/bin/env python3
"""Counterfactual B arithmetic, reading current upkeep table and read-only dev rows.
These are obligations, not a reconstructed time-series of all silver expenditure.
"""
import json,pathlib,re
HERE=pathlib.Path(__file__).resolve().parent;ROOT=HERE.parents[2]
text=(ROOT/'server/internal/combat/upkeep.go').read_text()
block=re.search(r'var UpkeepSpecs = map\[string\]UpkeepSpec\{(.*?)\n\}',text,re.S).group(1)
specs={k:{'grain':float(g),'silver':float(s)} for k,g,s in re.findall(r'"([a-z_]+)":\s*\{Grain:\s*([0-9.]+),\s*Silver:\s*([0-9.]+)\}',block)}
assert len(specs)==6 and specs['spearman']['silver']==1
def scenario(composition):
 before=sum(specs[k]['silver']*n for k,n in composition.items())
 removed=specs['spearman']['silver']*composition.get('spearman',0)
 return {'composition':composition,'gross_before':before,'gross_after':before-removed,'reduction_pct':100*removed/before if before else None,
 'own_payer_net_before':.3*before,'own_payer_net_after':.3*(before-removed),
 'grain_garrison':sum(specs[k]['grain']*n for k,n in composition.items())}
rows=[json.loads(line) for line in (HERE/'dev-snapshot.jsonl').read_text().splitlines() if line.startswith('{')]
gross=spear=net=spearnet=0
for row in rows:
 if row['kind']!='units':continue
 u=row['data'];rate=specs.get(u['type'],{}).get('silver',0)*(u['size']/100 if u['category']=='land' else 1)*u['units']
 # Active founders charge their store's saved rate; here only measure current-table obligations.
 f=.3 if u['own_payer'] and not u['active_founder'] else 1
 gross+=rate;net+=rate*f
 if u['type']=='spearman':spear+=rate;spearnet+=rate*f
data={'specs':specs,'live_nominal_payroll':{'gross_before':gross,'gross_after':gross-spear,'net_before_default_share':net,'net_after_default_share':net-spearnet,'spearman_pct':100*spear/gross if gross else None},
 'live_saved_founder_rates':[r['data'] for r in rows if r['kind']=='founders'],
 'scenarios':{name:scenario(c) for name,c in {
 'inland_start':{'spearman':2},'coastal_start':{'spearman':2,'galley':1},
 'inland_plus_elite':{'spearman':2,'elite_infantry':1},
 'ten_spears_elite_chariot':{'spearman':10,'elite_infantry':1,'war_chariot':1},
 'two_spears_each_professional_type':{'spearman':2,'elite_infantry':1,'war_chariot':1,'galley':1,'war_galley':1,'merchantman':1}}.items()}}
(HERE/'payroll.json').write_text(json.dumps(data,indent=2)+'\n')
print(json.dumps(data,indent=2))
