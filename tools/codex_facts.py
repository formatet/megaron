#!/usr/bin/env python3
"""Render catalogue facts into Markdown and one article bundle. --check is read-only."""
from pathlib import Path
import json,re,sys
ROOT=Path(__file__).resolve().parents[1];D=ROOT/'web/static/codex';reference=json.loads((D/'reference.json').read_text());check='--check' in sys.argv
units={x['type']:x for x in reference['units']};buildings={x['type']:x for x in reference['buildings']};changed=[]
def amount(x):return f'{x:g}'
def days(x):return amount(x)+' game '+('day' if x==1 else 'days')
def costs(values,linked=False):
 order=['grain','timber','cedar','stone','bronze','copper','tin','silver']
 return ' · '.join(amount(values[k])+' '+('[[bronze]]' if linked and k=='bronze' else k)+(' (or cedar)' if k=='timber' else '') for k in [*order,*sorted(set(values)-set(order))] if k in values and values[k])
def block(kind,key):
 if kind=='building':
  x=buildings[key];rows=[('Cost, level 1',costs(x['costs'])),('Build time, level 1',days(x['duration_game_days'])),('Maximum level',str(x['max_level']))]
  for level,price in x.get('upgrade_costs',{}).items():rows.append(('Cost to level '+level,costs(price)))
  if x.get('requires_coastal'):rows.append(('Site','The city must border navigable water.'))
  if x.get('requires_deposits'):rows.append(('Deposit',' or '.join(x['requires_deposits'])))
  if x.get('hex_bound'):rows.append(('Location','One selected catchment hex.'))
  for recipe in reference['recipes']:
   if recipe['building_type']==key:rows.append(('Recipe',' + '.join(amount(i['quantity'])+' '+i['good_key'] for i in recipe['ingredients'])+' → '+amount(recipe['output_qty'])+' '+recipe['output_key']))
 elif kind=='prayer':
  x=reference['prayers'][key];rows=[('God',x['God']),('Minimum standing',amount(x['MinKharis'])+' Kharis'),('Traditional offering',costs(x['Offering'])),('God’s waiting time',days(x['CooldownTicks']))]
 else:raise ValueError((kind,key))
 return '| Reference | Current rule |\n|---|---|\n'+'\n'.join('| '+a+' | '+b+' |' for a,b in rows)
def write(path,text):
 if not path.exists() or path.read_text()!=text:
  changed.append(str(path.relative_to(ROOT)))
  if not check:path.write_text(text)
index=json.loads((D/'index.json').read_text());bodies={}
for article in index['articles']:
 path=D/(article['id']+'.md');text=path.read_text()
 text=re.sub(r'<!-- facts: (building|prayer) ([a-z0-9_]+) -->[\s\S]*?<!-- /facts -->',lambda m:'<!-- facts: '+m[1]+' '+m[2]+' -->\n'+block(m[1],m[2])+'\n<!-- /facts -->',text)
 for entity in article.get('entities',[]):
  if entity['kind']!='unit' or entity['key'] not in units:continue
  key=entity['key'];x=units[key];price=costs(x['costs'],True)
  if 'silver' not in x['costs']:price+=' · '+('no timber, no silver' if key=='merchantman' else 'no silver')
  text=re.sub(r'^(\| \*\*Cost for one (?:cohort|ship)\*\* \| ).*?( \|)$',lambda m:m[1]+price+m[2],text,flags=re.M)
  text=re.sub(r'^(\| \*\*(?:Training|Build time)\*\* \| ).*?( \|)$',lambda m:m[1]+days(x['duration_game_days'])+m[2],text,flags=re.M)
 write(path,text);bodies[article['id']]=text
write(D/'articles.json',json.dumps(bodies,ensure_ascii=False,indent=2)+'\n')
if changed:
 print(('STALE: ' if check else 'Updated: ')+', '.join(changed))
 if check:sys.exit(1)
else:print('Codex facts and article bundle: current')
