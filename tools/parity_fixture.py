#!/usr/bin/env python3
"""Explicit content-stress fixture in the isolated acceptance world.
Normal register/join/found APIs; SQL enriches only the dedicated fixture city and
sets its blessing/resources. This proves layout, not normal progression.
"""
import json,pathlib,re,subprocess,urllib.error,urllib.request
ROOT=pathlib.Path(__file__).resolve().parents[1];OUT=ROOT/'docs/reviews/paritet-20261011';BASE='http://localhost:8097';PW='acceptance-pw-123'
def api(tail,token=None,data=None):
 req=urllib.request.Request(BASE+'/api/v1/'+tail,headers={'Content-Type':'application/json',**({'Authorization':'Bearer '+token} if token else {})},data=None if data is None else json.dumps(data).encode())
 return json.load(urllib.request.urlopen(req,timeout=30))
def player(name,join=True):
 try:t=api('auth/login',data={'username_or_email':name,'password':PW})['access_token']
 except urllib.error.HTTPError:
  t=api('auth/register',data={'username':name,'password':PW})['access_token']
  if join:api('worlds/'+world+'/join',t,{})
 return t
health=json.load(urllib.request.urlopen(BASE+'/healthz'))
expected=subprocess.check_output(['git','-C',str(ROOT),'rev-parse','--short=7','HEAD'],text=True).strip()
assert health['commit']==expected and health['migration']==171,health
world=api('worlds')[0]['id']
nomad=player('ParityNomad');rich=player('ParityRich')
state=api('worlds/'+world+'/founding/status',rich)
if state.get('active'):
 try:api('worlds/'+world+'/founding/settle',rich,{'name':'Parity Rich Mycenae','culture':'achaean'})
 except urllib.error.HTTPError as e:raise RuntimeError(e.read().decode())
# Founding phase key may vary: if no owned settlement, normal settle is required.
provinces=api('worlds/'+world+'/provinces',rich)
owned=[p for p in provinces if p.get('own') and p.get('settlement_id')]
if not owned:
 api('worlds/'+world+'/founding/settle',rich,{'name':'Parity Rich Mycenae','culture':'achaean'})
 provinces=api('worlds/'+world+'/provinces',rich);owned=[p for p in provinces if p.get('own') and p.get('settlement_id')]
p=owned[0];sid=p['settlement_id'];pid=api('auth/me',rich)['id']
btypes=re.findall(r'Building\w+\s+BuildingType\s*=\s*"([a-z_]+)"',(ROOT/'server/internal/province/model.go').read_text())
# Read the province's authoritative coastal flag. Maritime workplaces are
# omitted from this fixture; the same 30-row layout is used on either coast.
coastal=api('worlds/'+world+'/provinces/'+p['id'],rich)['coastal']
btypes=[b for b in btypes if b not in ['harbour','shipyard']]
build=[]
for b in btypes:
 bound=b in ['farm','mine','lumbermill','stonequarry']
 build.append(f"('{sid}','{b}',3,{p['q'] if bound else 'NULL'},{p['r'] if bound else 'NULL'})")
# Add all catchment farms to force long Buildings content using valid hex-bound rows.
for dq in range(-2,3):
 for dr in range(-2,3):
  if max(abs(dq),abs(dr),abs(dq+dr))<=2 and (dq or dr):
   build.append(f"('{sid}','farm',2,{p['q']+dq},{p['r']+dr})")
unit_values=[]
for i in range(12):
 typ=['spearman','elite_infantry','war_chariot'][i%3]
 unit_values.append(f"('{world}','{pid}','{typ}','land',100,'garrison','{sid}','{sid}',{p['q']},{p['r']},'Parity Long Named Bronze Guard {i+1}',{i+1})")
bless=(ROOT/'server/internal/economy/harvest_blessing.go').read_text();assert 'HarvestBlessingYieldBonus' in bless
sql=f"""BEGIN;
DELETE FROM buildings WHERE settlement_id='{sid}';
INSERT INTO buildings(settlement_id,building_type,level,hex_q,hex_r) VALUES {','.join(build)};
DELETE FROM units WHERE owner_id='{pid}' AND name LIKE 'Parity Long Named Bronze Guard %';
INSERT INTO units(world_id,owner_id,type,category,size,status,settlement_id,home_settlement_id,q,r,name,ordinal) VALUES {','.join(unit_values)};
UPDATE settlements SET population=10000, harvest_blessing_until_tick=(SELECT current_tick+2000 FROM worlds WHERE id='{world}') WHERE id='{sid}';
UPDATE settlement_goods SET amount=100000,calc_tick=(SELECT current_tick FROM worlds WHERE id='{world}') WHERE settlement_id='{sid}';
UPDATE settlements SET harvest_blessing_until_tick=(SELECT current_tick+2000 FROM worlds WHERE id='{world}') WHERE name='Mycenae' AND world_id='{world}';
COMMIT;"""
(OUT/'fixture.sql').write_text(sql+'\n');subprocess.run(['tools/acceptance.sh','psql',sql],cwd=ROOT,check=True,capture_output=True)
sett=api('worlds/'+world+'/provinces/'+p['id'],rich)
(OUT/'rich-city.json').write_text(json.dumps(sett,indent=2)+'\n')
(OUT/'fixture.json').write_text(json.dumps({'world':world,'rich_player':'ParityRich','host_player':'ParityNomad','rich_city':p,'coastal':coastal,'buildings':len(sett['settlement']['buildings']),'fixture_garrison':12,'setup':'normal register/join/found; explicit SQL content-stress + active blessing; not normal gameplay progression'},indent=2)+'\n')
print('rich city fixture:',len(sett['settlement']['buildings']),'buildings, 12 named cohorts')
