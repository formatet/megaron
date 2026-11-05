#!/usr/bin/env python3
"""Render catalogue facts into Markdown and one article bundle. --check is read-only."""
from pathlib import Path
import json,re,sys
ROOT=Path(__file__).resolve().parents[1];D=ROOT/'web/static/codex';reference=json.loads((D/'reference.json').read_text());check='--check' in sys.argv
units={x['type']:x for x in reference['units']};buildings={x['type']:x for x in reference['buildings']};changed=[]
def amount(x):return f'{x:g}'
def days(x):return amount(x)+' game '+('day' if x==1 else 'days')
def costs(values,linked=False):
 # Alphabetical by good: a cost is an enumeration with no inherent order.
 return ' · '.join(amount(values[k])+' '+('[[bronze]]' if linked and k=='bronze' else k)+(' (or cedar)' if k=='timber' else '') for k in sorted(values) if values[k])
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
  x=reference['prayers'][key];rows=[('God',x['God']),('Minimum standing',amount(x['MinKharis'])+' Kharis'),('Traditional offering',costs(x['Offering'])),('Temple’s rest (level 1, traditional gift)',days(x['CooldownTicks']))]
 else:raise ValueError((kind,key))
 return '| Reference | Current rule |\n|---|---|\n'+'\n'.join('| '+a+' | '+b+' |' for a,b in rows)
def favours():
 # Each prayer's taste from reference.json (religion.FavoursFor), dearest first; prayers alphabetical by name.
 rows=[]
 for x in sorted(reference['prayers'].values(),key=lambda x:x['Name']):
  f=x.get('Favours') or {}
  taste=', '.join(k+' ×'+amount(v) for k,v in sorted(f.items(),key=lambda kv:(-kv[1],kv[0]))) if f else 'no preference: every good at its worth'
  rows.append('| '+x['Name']+' | '+x['God']+' | '+taste+' |')
 return '| Prayer | God | Favours (dearest first) |\n|---|---|---|\n'+'\n'.join(rows)
def kharis_prayers():
 # Prayers alphabetical by name; linked to their article when the index names one for the prayer.
 art={e['key']:a['id'] for a in index['articles'] for e in a.get('entities',[]) if e['kind']=='prayer'}
 rows=[]
 for k,x in sorted(reference['prayers'].items(),key=lambda kv:kv[1]['Name']):
  name=('[['+art[k]+'|'+x['Name']+']]') if k in art else x['Name']
  rows.append('| '+name+' | '+x['God']+' | '+amount(x['MinKharis'])+' | '+costs(x['Offering'])+' | '+' / '.join(amount(r) for r in x['RestByLevel'])+' game days |')
 return '| Prayer | God | Required kharis | Traditional offering | Temple’s rest at level 1 / 2 / 3 |\n|---|---|---|---|---|\n'+'\n'.join(rows)
def kharis_demand():
 k=reference['kharis'];rows=[]
 for d in k['demand']:
  up=k['per_demand']*d['demand'];rows.append('| '+str(d['cities'])+' | '+amount(d['demand'])+f' | {up:.2f} | {up/k["per_temple_weight"]:.2f} |')
 return '| Active cities | Realm demand | Upkeep per game day | Temple weight to hold steady |\n|---|---|---|---|\n'+'\n'.join(rows)
def pct(x):return f'{x*100:g}%'
def kharis_standing():
 k=reference['kharis']
 rows=[(amount(k['floor']),'The floor: kharis never falls below this.'),
  ('below '+amount(k['wrathful_threshold']),pct(k['wrathful_probability'])+' chance of punishment each game day.'),
  ('below '+amount(k['wrath_threshold']),pct(k['wrath_probability'])+' chance of punishment each game day.'),
  (amount(k['wrath_threshold'])+' and above','No punishment.'),
  (amount(k['starting']),'A new Wanax starts here. It is also the ceiling with no temple.'),
  (amount(k['bless_threshold'])+' and above','With a maintained temple: '+pct(k['bless_probability'])+' chance of a spontaneous blessing each game day.')]
 for l in k['levels']:rows.append((amount(l['ceiling']),'The ceiling set by a level '+str(l['level'])+' temple.'))
 return '| Kharis | What it means |\n|---|---|\n'+'\n'.join('| '+a+' | '+b+' |' for a,b in rows)
def temple_levels():
 k=reference['kharis'];rows=[]
 for l in k['levels']:
  r=l['weight']*k['per_temple_weight']
  tithe=' + '.join(amount(p['amount'])+' '+p['good']+' ('+pct(p['share'])+')' for p in l['tithe'])
  rows.append('| '+str(l['level'])+' | '+str(l['workplaces'])+' | '+amount(l['weight'])+f' | {r:.2f} | {r*k["rich_offering_bonus"]:.2f} | '+amount(l['ceiling'])+' | '+tithe+' |')
 return '| Level | Workplaces | Weight | Kharis per game day, full staff and tithe | With rich offering | Kharis ceiling | Daily tithe |\n|---|---|---|---|---|---|---|\n'+'\n'.join(rows)
def tastes():
 # Each god's rich-offering taste from reference.json (religion.TempleGods), dearest first; gods alphabetical.
 rows=[]
 for g in sorted(reference['gods'],key=lambda g:re.sub(r'^the ','',g['Name'].lower())):
  t=g.get('Tastes') or {}
  rows.append('| '+g['Name'][0].upper()+g['Name'][1:]+' | '+(', '.join('[['+k+'|'+k+']] ×'+amount(v) for k,v in sorted(t.items(),key=lambda kv:(-kv[1],kv[0]))) or 'no preference')+' |')
 return '| God | Prizes in a rich offering |\n|---|---|\n'+'\n'.join(rows)
def write(path,text):
 if not path.exists() or path.read_text()!=text:
  changed.append(str(path.relative_to(ROOT)))
  if not check:path.write_text(text)
index=json.loads((D/'index.json').read_text());bodies={}
def summary(article):
 # One line for a hub list: the index's `summary`, else the article's first prose sentence.
 if article.get('summary'):return article['summary']
 text=re.sub(r'<!--[\s\S]*?-->','',(D/(article['id']+'.md')).read_text())
 para=next((p.strip() for p in text.split('\n\n') if p.strip() and not p.strip().startswith(('|','#','-','>'))),'')
 para=re.sub(r'\[\[[^\]|]+\|([^\]]+)\]\]',r'\1',para);para=re.sub(r'\[\[([^\]]+)\]\]',r'\1',para).replace('**','')
 m=re.match(r'(.+?[.!?])(\s|$)',para);return m[1] if m else para
def members(kind,keys):
 out=[a for a in index['articles'] if not a.get('future') and any(e['kind']==kind and (not keys or e['key'] in keys) for e in a.get('entities',[]))]
 if keys:out.sort(key=lambda a:min(keys.index(e['key']) for e in a['entities'] if e['key'] in keys))
 else:out.sort(key=lambda a:re.sub(r'^the ','',a['title'].lower()))  # hub lists are alphabetical by displayed title
 return out
def hub_list(kind,keys):
 # A hub lists every member of its family, so no unit, building or good is ever left unlinked.
 return '| Name | In short |\n|---|---|\n'+'\n'.join('| [['+a['id']+']] | '+summary(a).replace('|','/')+' |' for a in members(kind,keys))
for article in index['articles']:
 path=D/(article['id']+'.md');text=path.read_text()
 text=re.sub(r'<!-- facts: (building|prayer) ([a-z0-9_]+) -->[\s\S]*?<!-- /facts -->',lambda m:'<!-- facts: '+m[1]+' '+m[2]+' -->\n'+block(m[1],m[2])+'\n<!-- /facts -->',text)
 text=re.sub(r'<!-- tastes -->[\s\S]*?<!-- /tastes -->',lambda m:'<!-- tastes -->\n'+tastes()+'\n<!-- /tastes -->',text)
 text=re.sub(r'<!-- favours -->[\s\S]*?<!-- /favours -->',lambda m:'<!-- favours -->\n'+favours()+'\n<!-- /favours -->',text)
 for name,fn in (('kharis-prayers',kharis_prayers),('kharis-demand',kharis_demand),('kharis-standing',kharis_standing),('temple-levels',temple_levels)):
  text=re.sub('<!-- '+name+r' -->[\s\S]*?<!-- /'+name+' -->',lambda m,name=name,fn=fn:'<!-- '+name+' -->\n'+fn()+'\n<!-- /'+name+' -->',text)
 text=re.sub(r'<!-- list: ([a-z]+)((?: [a-z0-9_]+)*) -->[\s\S]*?<!-- /list -->',lambda m:'<!-- list: '+m[1]+m[2]+' -->\n'+hub_list(m[1],m[2].split())+'\n<!-- /list -->',text)
 for entity in article.get('entities',[]):
  if entity['kind']!='unit' or entity['key'] not in units:continue
  key=entity['key'];x=units[key];price=costs(x['costs'],True)
  if 'silver' not in x['costs']:price+=' · '+('no silver, no timber' if key=='merchantman' else 'no silver')
  text=re.sub(r'^(\| \*\*Cost for one (?:cohort|ship)\*\* \| ).*?( \|)$',lambda m:m[1]+price+m[2],text,flags=re.M)
  text=re.sub(r'^(\| \*\*(?:Training|Build time)\*\* \| ).*?( \|)$',lambda m:m[1]+days(x['duration_game_days'])+m[2],text,flags=re.M)
 write(path,text);bodies[article['id']]=text
write(D/'articles.json',json.dumps(bodies,ensure_ascii=False,indent=2)+'\n')
if changed:
 print(('STALE: ' if check else 'Updated: ')+', '.join(changed))
 if check:sys.exit(1)
else:print('Codex facts and article bundle: current')
