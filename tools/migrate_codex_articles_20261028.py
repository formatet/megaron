#!/usr/bin/env python3
"""One-time, source-preserving article split. Original packets and destinations are recorded."""
from pathlib import Path
import json,re,subprocess,hashlib
ROOT=Path(__file__).resolve().parents[1];OUT=ROOT/'web/static/codex';REVIEW=ROOT/'docs/reviews/wiki-catalog';BASE='f45e4cbd'
def old(path):return subprocess.check_output(['git','-C',str(ROOT),'show',BASE+':web/static/codex/'+path],text=True)
index=json.loads(old('index.json'));baseline={a['id']:old(a['id']+'.md') for a in index['articles']};facts=json.loads((OUT/'reference.json').read_text());metadata={a['id']:dict(a) for a in index['articles']};bodies={};migration=[]
def slug(s):return re.sub(r'[^a-z0-9]+','-',s.lower()).strip('-')[:65].rstrip('-')
def visible(s):
 s=re.sub(r'<!--.*?-->','',s,flags=re.S);s=re.sub(r'\[\[([^]|]+)\|([^]]+)\]\]',r'\2',s);s=re.sub(r'\[\[([^]]+)\]\]',r'\1',s);return re.sub(r'[`*#|]','',s).strip()
def words(s):return len(visible(s).split())
def short_lead(s):
 line=next((x for x in s.splitlines() if x.strip() and not x.startswith(('<!--','|','```'))),'')
 line=re.sub(r'^[-\d. ]+','',line);v=visible(line);sent=re.split(r'(?<=[.!?])\s',v)[0]
 return sent if sent and len(sent.split())<=35 and sent.endswith(('.', '!', '?')) else ''
def atomise(body):
 for block in re.split(r'\n\s*\n',body.strip()):
  if not block.strip():continue
  if words(block)>170 and re.match(r'^(?:- |\d+\. )',block):
   yield from (x.strip() for x in re.split(r'\n(?=(?:- |\d+\. ))',block) if x.strip())
  elif words(block)>170 and block.startswith('|'):
   lines=block.splitlines();header='\n'.join(lines[:2]);yield from (header+'\n'+line for line in lines[2:] if line.strip())
  else:yield block.strip()
leads={
 'buildings':'Buildings provide workplaces, defences and the facilities needed for recruitment and trade.',
 'units':'Forces are the soldiers, ships and people you command.',
 'sea':'Ships carry people, messages and goods across navigable water.',
 'marching':'A march moves selected forces along a physical route to a destination.',
 'screen':'The map, drawers and dispatches show your realm and let you give orders.',
 'time':'A game day is one tick of the world; its real duration depends on the world.',
 'rites':'A rite is a request made at the temple of its god.',
 'kharis':'Kharis is your realm’s standing with the gods.',
 'sight':'Sight determines which ground and moving people you can currently see.',
 'roads':'A road is a built connection between neighbouring land hexes.',
 'sieges':'A siege threatens a city by holding the ground that feeds it.',
}
# Preserve notification/drawer ids as compact entry points, then link to details.
section_ids={('sea','Ships'):'ship-types',('sea','Carry troops to your own city'):'landing-in-own-city',('sea','At sea'):'ship-missions',('sea','Storms'):'sea-storm-risks',('sea','Damage and loss'):'ship-loss',('sea','Hitching a ride — how a letter or an order crosses the sea'):'runner-passage',('sea','What the map shows while it crosses'):'runner-passage-on-map',('sea',"A runner stuck waiting — your decision, not the game's"):'runner-waiting',('sea','Call it back — bring the runner home undelivered'):'runner-return',('sea','Arrange passage — send a ship for your own runner'):'arrange-passage',('sea','Fetch home — send a ship to fetch a unit'):'fetch-by-ship',('sea','Sailing time'):'sailing-time',('citizens','New people place themselves'):'free-citizens',('sight','Memory'):'map-memory',('sight','Live sight'):'live-sight',('sight','Moving actors on the map'):'map-actors',('sight','Exploring'):'exploration',('sight','Rivers'):'rivers',('buildings','On a hex'):'hex-buildings',('buildings','The rule'):'hex-production-buildings',('buildings','Levels'):'building-levels',('buildings','Demolishing and rebuilding'):'demolition',('upkeep','Pay — silver'):'military-pay',('upkeep','Food'):'military-food',('kharis','Upkeep of the cult'):'cult-upkeep',('kharis','How to raise it'):'raising-kharis',('rites','Standing is a requirement'):'prayer-requirements',('rites','The Minoan prayers'):'minoan-prayers',('rites','What an offering means'):'offerings',('temples','Devotion — people'):'temple-workers',('temples','Level'):'temple-levels',('temples','Offerings — local early goods'):'daily-offerings'}
entity_names={};entity_ids={}
units={'spearman':'spearmen','elite_infantry':'elite-infantry','war_chariot':'war-chariot','galley':'galley','war_galley':'war-galley','merchantman':'emporos','nomadic_host':'horde'}
buildings={'stonequarry':'stone-quarry','olive_press':'olive-press','silo':'granary','wall':'walls','temple':'temple'}
terrains={'forest_olive_grove':'olive-grove','mountain_limestone':'limestone-mountains','mountain_red':'red-mountains','coastal_sea':'coastal-sea','deep_sea':'deep-sea','river':'rivers','river_valley':'river-valley','river_delta':'river-delta','river_ford':'river-ford','scrub_maquis':'maquis','semi_desert':'semi-desert','forest_cedar':'cedar-forest'}
prayers={'minoan_potnia_vision_v2':'potnias-vision','minoan_potnia_walls':'mistress-of-the-walls','minoan_velchanos_green_shoots':'green-shoots','minoan_asterion_thread':'the-thread-shows-a-path','minoan_stillness_passage':'favourable-passage'}
lookup=json.loads((REVIEW/'entities-source-inventory.json').read_text())
for kind,key,ids,category in [('unit','units',units,'War'),('building','buildings',buildings,'Your city'),('terrain','terrains',terrains,'The wider world'),('god','gods',{'sea_daimons_stillness':'sea-daimons'},'The gods'),('prayer','prayers',prayers,'The gods')]:
 for row in lookup[key]:
  if kind=='prayer' and row.get('status')!='current_mvp':continue
  aid=ids.get(row['id'],row['id'].replace('_','-'));entity_names[(kind,row['id'])]=row['name'];entity_ids[(kind,row['id'])]=aid
  metadata.setdefault(aid,{'id':aid,'title':row['name'],'category':category,'related':[]})
  metadata[aid].setdefault('entities',[]).append({'kind':kind,'key':row['id']})
for row in facts['goods']:
 if row['status']!='active' and row['key']!='shipbuilding':continue
 aid={'cult':'devotion'}.get(row['key'],row['key']);key=row['key'];entity_ids[('good',key)]=aid;entity_names[('good',key)]=row['name'];metadata.setdefault(aid,{'id':aid,'title':row['name'] if key!='cult' else 'Devotion','category':'Goods and trade','related':[]});metadata[aid].setdefault('entities',[]).append({'kind':'good','key':key})

def add(aid,text,source):
 bodies.setdefault(aid,[])
 if text not in bodies[aid]:bodies[aid].append(text)
 migration.append({**source,'home':aid,'text':text,'sha256':hashlib.sha256(text.encode()).hexdigest()})

def make_details(parent,heading,packets):
 base=section_ids.get((parent,heading),parent+'-'+slug(heading or 'details'))
 # A stable, descriptive first-phrase title is refined in titles.json after splitting.
 batch=[];n=0;homes=[]
 def flush():
  nonlocal batch,n
  if not batch:return
  aid=base if n==0 else base+'-'+str(n+1);n+=1;homes.append(aid)
  title=heading or metadata[parent]['title']+' details'
  if n>1:
   phrase=visible(batch[0][0]).split('\n')[0];m=re.search(r'\*\*([^*]+)\*\*',batch[0][0]);phrase=m.group(1) if m else phrase
   phrase=re.sub(r'^[-\d. ]+','',phrase);title=phrase[:85].rstrip(' ,:;')
  if aid not in metadata:metadata[aid]={'id':aid,'title':title,'category':metadata[parent]['category'],'related':[parent]}
  for packet,source in batch:add(aid,packet,source)
  batch=[]
 for packet,source in packets:
  if batch and sum(words(p) for p,_ in batch)+words(packet)>190:flush()
  batch.append((packet,source))
 flush();return homes

for aid,text in baseline.items():
 # Existing short entity articles stay intact; costs are validated/generated separately.
 if words(text)<=235:
  add(aid,text,{'old_article':aid,'section':'all'});continue
 parts=re.split(r'^## (.+)$',text,flags=re.M);sections=[('',parts[0])]+[(parts[i],parts[i+1]) for i in range(1,len(parts),2)]
 primary=[];new_homes=[]
 for heading,body in sections:
  packets=[(p,{'old_article':aid,'section':heading or 'opening'}) for p in atomise(body)]
  if not heading:
   while packets and sum(words(x[0]) for x in primary)+words(packets[0][0])<=110:primary.append(packets.pop(0))
  if packets:new_homes.extend(make_details(aid,heading,packets))
 if aid in leads:add(aid,leads[aid],{'old_article':aid,'section':'new entry definition'})
 for packet,source in primary:add(aid,packet,source)
 if not bodies.get(aid):add(aid,short_lead(text) or metadata[aid]['title']+'.',{'old_article':aid,'section':'entry definition'})
 metadata[aid]['related']=list(dict.fromkeys([*new_homes,*metadata[aid].get('related',[])]))
 if new_homes:bodies[aid].append('Read more: '+', '.join('[['+home+']]' for home in new_homes)+'.')

# Short, code-derived reference articles for objects absent from the old index.
build_by={x['type']:x for x in facts['buildings']}
goods_intro={
 'grain':'Grain is a food good produced by citizens working suitable fields.', 'fish':'Fish is a food good caught by citizens working fishing places.',
 'oil':'Oil is a food and offering good refined at an Olive Press.', 'wine':'Wine is a food and offering good refined at a Winery.',
 'livestock':'Livestock is the herd kept by a city and used as emergency food.', 'timber':'Timber is building material gathered by citizens on suitable wooded ground.',
 'cedar':'Cedar is scarce wood used for ships and advanced building upgrades.', 'stone':'Stone is building material quarried from land hexes.',
 'copper':'Copper is an ore used with tin to make bronze.', 'tin':'Tin is the scarce ore needed with copper to make bronze.',
 'cult':'Devotion is work performed by temple workers, not a stockpiled commodity.', 'shipbuilding':'Shipbuilding is service work performed at a Shipyard, not cargo stored in a warehouse.',
}
terrain_intro={
 'plains':'Plains are open land suitable for grain fields.', 'forest_olive_grove':'An olive grove is wooded land that supports timber and the city’s olive economy.',
 'hills':'Hills are land with grain, stone and possible mineral deposits.', 'mountain_limestone':'Limestone mountains are difficult ground with strong stone-quarrying potential.',
 'mountain_red':'Red mountains are difficult ground that may contain strategic ore deposits.', 'coastal_sea':'Coastal sea is navigable water beside land, with fishing places.',
 'deep_sea':'Deep sea is open navigable water; fishing there requires a Harbour.', 'river':'A river is flowing navigable water that affects movement and the coast rule.',
 'river_valley':'A river valley is land beside a river, suitable for settlement and production.', 'river_delta':'A river delta is productive land near a river’s mouth.',
 'river_ford':'A ford is shallow river ground where land travel can cross the water.', 'scrub_maquis':'Maquis is Mediterranean scrub that can provide timber.',
 'semi_desert':'Semi-desert is dry ground with limited food and costly land travel.', 'forest_cedar':'A cedar forest is wooded ground where citizens can gather cedar.',
}
for (kind,key),aid in entity_ids.items():
 if bodies.get(aid):continue
 title=entity_names[(kind,key)]
 if kind=='building':
  role=build_by[key]['purpose'];intro=f'{title} is a building: '+role[0].lower()+role[1:]+'.'
  bodies[aid]=[intro,'Build or upgrade it in **City → Buildings → Construct**, or use `keryx build --type '+key+'`'+(' with `--hex` to choose its catchment hex.' if build_by[key].get('hex_bound') else '.'),'<!-- facts: building '+key+' -->','<!-- /facts -->','See [[buildings]] and [[citizens]].']
 elif kind=='terrain':bodies[aid]=[terrain_intro[key],'Select a known hex to inspect its ground and available production. City placement shows its actual workplaces and the next worker’s yield; movement previews show the route’s actual arrival time. In Keryx, inspect the ground with `hex`.','See [[catchment]], [[citizens]] and [[marching]].']
 elif kind=='god':
  prayer_ids=[prayers[x] for x,p in facts['prayers'].items() if p['GodID']==key];bodies[aid]=[title+' is a god whose temple opens prayers for your cities.','Build a temple for this god in **City → Buildings → Construct** or use `keryx build --type temple --god '+key+'`. Temple workers and daily offerings maintain realm-wide [[kharis]].','Prayers: '+', '.join('[['+x+']]' for x in prayer_ids)+'.','See [[temples]], [[rites]] and [[offerings]].']
 elif kind=='prayer':
  prayer=facts['prayers'][key];bodies[aid]=[prayer['Name']+' is a prayer to '+prayer['God']+'.',prayer['Description'],'<!-- facts: prayer '+key+' -->','<!-- /facts -->','Offer in **Kult → Prayers** at the matching city and god, or use `keryx rite --prayer '+key+'`. A valid composed offering can replace the traditional recipe.','See [[prayer-requirements]], [[offerings]] and [[temples]].']
 elif kind=='good':
  bodies[aid]=[goods_intro.get(key,title+' is a good used by your cities.'),'Use **Economy → Goods** or `keryx goods` to inspect actual stocks and flow. '+('Assign workers through City placement or `keryx staff`.' if key in ('cult','shipbuilding') else 'Send available cargo through Transfer or `keryx transfer --good '+key+' --qty AMOUNT --dest CITY`.'),'See [[goods]], [[citizens]] and '+('[[temples]].' if key=='cult' else '[[shipyard]].' if key=='shipbuilding' else '[[trade]].')]
# Every entity stays reachable even if its original concept article did not name it.
for kind,parent in [('unit','units'),('building','buildings'),('terrain','sight'),('god','temples'),('prayer','rites'),('good','goods')]:
 refs=list(dict.fromkeys(aid for (k,_),aid in entity_ids.items() if k==kind));metadata[parent]['related']=list(dict.fromkeys(metadata[parent].get('related',[])+refs))
# Search aliases are metadata, not retired labels shown as prose.
for (kind,key),aid in entity_ids.items():
 aliases=[key.replace('_',' ')];
 if kind=='unit' and key in ('galley','war_galley','merchantman'):aliases+=['ship','boat']
 metadata[aid]['aliases']=list(dict.fromkeys(metadata[aid].get('aliases',[])+aliases))
for aid,aliases in {'kharis':['mana','divine standing'],'units':['units','army','forces'],'horde':['nomadic host','horde'],'time':['game day','game days','tick'],'elite-infantry':['elite infantry','bronze guard'],'war-chariot':['war chariot','chariot'],'emporos':['merchantman','merchant ship'],'harbour':['harbor','port']}.items():metadata[aid]['aliases']=list(dict.fromkeys(metadata[aid].get('aliases',[])+aliases))
# Parent links are deduped and self-links removed.
for aid,a in metadata.items():a['related']=[x for x in dict.fromkeys(a.get('related',[])) if x!=aid]
for aid,packets in bodies.items():(OUT/(aid+'.md')).write_text('\n\n'.join(packets).strip()+'\n')
index['articles']=list(metadata.values());(OUT/'index.json').write_text(json.dumps(index,ensure_ascii=False,indent=2)+'\n')
(REVIEW/'fact-migration.json').write_text(json.dumps({'baseline':BASE,'records':migration},ensure_ascii=False,indent=2)+'\n')
(REVIEW/'article-sizes.json').write_text(json.dumps(sorted([{'id':aid,'title':metadata[aid]['title'],'words':words('\n\n'.join(body))} for aid,body in bodies.items()],key=lambda x:-x['words']),ensure_ascii=False,indent=2)+'\n')
print(len(metadata),'articles;',len(migration),'preserved packets')
