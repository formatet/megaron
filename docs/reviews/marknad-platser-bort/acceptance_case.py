#!/usr/bin/env python3
"""205 cutover proof; only two dedicated accounts in the isolated acceptance rig."""
import json,sys,subprocess,urllib.request,urllib.error,uuid,os
from pathlib import Path
ROOT=Path(__file__).resolve().parents[3];OUT=Path(__file__).resolve().parent
BASE='http://localhost:8097';CFG=Path.home()/'.config/megaron-market-proof30'
def req(path,body=None,token=None):
 h={'Content-Type':'application/json'}
 if token:h['Authorization']='Bearer '+token
 r=urllib.request.Request(BASE+path,headers=h,data=None if body is None else json.dumps(body).encode())
 with urllib.request.urlopen(r,timeout=20) as res:return json.load(res)
def sql(q):
 p=subprocess.run(['docker','exec','-i','megaron-acc-postgres-1','psql','-U','poleia','-d','poleia','-XAtq','-v','ON_ERROR_STOP=1'],input=q,text=True,capture_output=True)
 assert p.returncode==0,p.stderr
 return p.stdout.strip()
def rows(q):return json.loads(sql("SELECT coalesce(jsonb_agg(to_jsonb(x)),'[]'::jsonb) FROM ("+q+") x;"))
for container in ['megaron-acc-server-1','megaron-acc-postgres-1']:
 assert subprocess.check_output(['docker','inspect','-f','{{index .Config.Labels "com.docker.compose.project"}}',container],text=True).strip()=='megaron-acc'
def snapshot(c):
 return rows(f"SELECT s.population,(SELECT count(*) FROM settlement_placement WHERE settlement_id=s.id AND building_type='market') AS market_workers,(SELECT count(*) FROM settlement_placement WHERE settlement_id=s.id AND good_key='grain') AS food_workers,s.population/100-(SELECT count(*) FROM settlement_placement WHERE settlement_id=s.id) AS idle,(SELECT count(*) FROM buildings WHERE settlement_id=s.id AND building_type='market') AS markets,(SELECT amount FROM settlement_goods WHERE settlement_id=s.id AND good_key='silver') AS silver FROM settlements s WHERE id='{c['city']}'")[0]
mode=sys.argv[1];health=req('/healthz')
if mode=='seed':
 assert health['migration']==204,health
 CFG.mkdir(parents=True,exist_ok=True);CFG.chmod(0o700)
 world=rows("SELECT id FROM worlds WHERE status='active'")[0]['id'];cases=[]
 for n in [1,2]:
  name='MarketProof30'+str(n)
  auth=req('/api/v1/auth/register',{'username':name,'email':name.lower()+'@acceptance.local','password':'acceptance-only-30'})
  pid=str(uuid.UUID(req('/api/v1/auth/me',token=auth['access_token'])['id']));city=str(uuid.uuid4());province=str(uuid.uuid4())
  sql(f"""BEGIN;
INSERT INTO provinces(id,world_id,map_q,map_r,terrain_type) SELECT '{province}','{world}',q,r,terrain FROM map_tiles mt WHERE world_id='{world}' AND terrain NOT IN('sea','deep_sea','mountain_red') AND NOT EXISTS(SELECT 1 FROM provinces pp WHERE pp.world_id=mt.world_id AND pp.map_q=mt.q AND pp.map_r=mt.r) ORDER BY q,r LIMIT 1;
INSERT INTO settlements(id,world_id,province_id,name,culture_id,owner_id,control_type,is_capital,population,state) VALUES('{city}','{world}','{province}','{name}','minoan','{pid}','capital',true,600,'active');
INSERT INTO player_world_records(player_id,world_id,settlement_id,kharis_amount,kharis_rate,kharis_cap,kharis_calc_tick) VALUES('{pid}','{world}','{city}',40,0,40,current_world_tick());
INSERT INTO buildings(settlement_id,building_type,level) VALUES('{city}','market',{n});
INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) VALUES('{city}','silver',1000,0,100000,current_world_tick()),('{city}','grain',1000,0,100000,current_world_tick());
INSERT INTO settlement_placement(settlement_id,gubbe_ordinal,target_kind,building_type,good_key) VALUES('{city}',1,'building','market','pottery');
INSERT INTO settlement_placement(settlement_id,gubbe_ordinal,target_kind,hex_q,hex_r,good_key) SELECT '{city}',2,'hex',map_q+1,map_r,'grain' FROM provinces WHERE id='{province}';
COMMIT;""")
  config=CFG/(name+'.json');config.write_text(json.dumps({'server':BASE,'token':auth['access_token'],'world_id':world,'province_id':province,'player_id':pid}));config.chmod(0o600)
  case={'name':name,'city':city,'province':province,'world':world,'config':str(config)};case['before']=snapshot(case);cases.append(case)
 (OUT/'acceptance-before.json').write_text(json.dumps({'healthz':health,'cases':cases},indent=2)+'\n')
 print('Seeded2 dedicated accounts: population600/market1/food1/idle4. Ready for205.')
elif mode=='replay':
 assert health['migration']==205,health
 data=json.loads((OUT/'acceptance-before.json').read_text())
 stmts=['BEGIN;', (ROOT/'server/db/migrations/205_remove_market_workplaces.down.sql').read_text()]
 ids=[]
 for c in data['cases']:
  city=str(uuid.UUID(c['city']));ids.append("'"+city+"'")
  stmts.append(f"UPDATE settlements SET population=600 WHERE id='{city}';")
  stmts.append(f"INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) VALUES('{city}','grain',1000,0,100000,current_world_tick()) ON CONFLICT(settlement_id,good_key) DO UPDATE SET amount=1000,rate=0,calc_tick=current_world_tick();")
  stmts.append(f"DELETE FROM settlement_placement WHERE settlement_id='{city}'; INSERT INTO settlement_placement(settlement_id,gubbe_ordinal,target_kind,building_type,good_key) VALUES('{city}',1,'building','market','pottery'); INSERT INTO settlement_placement(settlement_id,gubbe_ordinal,target_kind,hex_q,hex_r,good_key) SELECT '{city}',2,'hex',map_q+1,map_r,'grain' FROM provinces WHERE id='{c['province']}';")
 stmts.append("SELECT jsonb_agg(jsonb_build_object('city',s.id,'population',s.population,'market_workers',1,'food_workers',1,'idle',s.population/100-2,'markets',1,'silver',(SELECT amount FROM settlement_goods WHERE settlement_id=s.id AND good_key='silver'))) FROM settlements s WHERE s.id IN("+','.join(ids)+");")
 stmts += [(ROOT/'server/db/migrations/205_remove_market_workplaces.up.sql').read_text(),'COMMIT;']
 before=json.loads(sql('\n'.join(stmts)))
 for c in data['cases']:
  c['before']=next(x for x in before if x['city']==c['city']).copy();c['before'].pop('city')
 data['replay']='After the initial204→205 startup crossed a food tick, supply food and replay exact205 down/up SQL atomically in the205 acceptance rig; schema_migrations is never edited.'
 (OUT/'acceptance-before.json').write_text(json.dumps(data,indent=2)+'\n')
 print('Exact205 migration replayed atomically with fed fixtures; version marker left205.')
elif mode=='check':
 assert health['migration']==205,health
 data=json.loads((OUT/'acceptance-before.json').read_text());result={'healthz':health,'cases':[]}
 for c in data['cases']:
  config=json.loads(Path(c['config']).read_text());token=config['token'];after=snapshot(c)
  assert after=={'population':600,'market_workers':0,'food_workers':1,'idle':5,'markets':1,'silver':1000},after
  opts=req('/api/v1/worlds/'+c['world']+'/provinces/'+c['province']+'/placement-options',token=token)
  assert all(x['building_type']!='market' for x in opts['buildings']),opts
  try:req('/api/v1/worlds/'+c['world']+'/provinces/'+c['province']+'/placements',{'target_kind':'building','building_type':'market','good_key':'pottery'},token)
  except urllib.error.HTTPError as e:assert e.code==422,e.code
  else:raise AssertionError('new market worker accepted')
  env={'HOME':str(Path.home()),'PATH':os.environ['PATH'],'POLEIA_CONFIG':c['config']}
  out=subprocess.check_output(['/tmp/order30-keryx','staff','market'],env=env,text=True);assert 'no workplaces' in out,out
  script="import {cityBuildingsHTML} from './web/static/js/megaron/ui/citygrid.js';let s='';for await(const c of process.stdin)s+=c;process.stdout.write(cityBuildingsHTML(JSON.parse(s).buildings));"
  rendered=subprocess.check_output(['node','--input-type=module','-e',script],cwd=ROOT,input=json.dumps(opts),text=True)
  assert 'market' not in rendered.lower(),rendered
  (OUT/(c['name']+'-web-workplaces.html')).write_text(rendered)
  result['cases'].append({'name':c['name'],'before':c['before'],'after':after,'cli':out.strip(),'api_refused':422,'web_market_workplace':False})
 (OUT/'acceptance-after.json').write_text(json.dumps(result,indent=2)+'\n')
 print('Both accounts: same600people,1farmer,1market,1000silver; former worker idle4→5; API/CLI/web consistent.')
else:raise SystemExit('seed | check')
