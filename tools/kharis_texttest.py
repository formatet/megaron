#!/usr/bin/env python3
"""ORDER29 replay of ORDER27 fixtures. Only megaron-acc / localhost:8097. No game-code edits."""
import json,os,subprocess,sys,time,urllib.request,urllib.error,uuid
from pathlib import Path
ROOT=Path(__file__).resolve().parents[1]
OUT=ROOT/'docs/reviews/kharistext-fixar'
BASE='http://localhost:8097'
CFG=Path.home()/'.config/poleia-texttest'
BIN=Path.home()/'.local/share/megaron-texttest/keryx'
NAMES=['Eumelos','Phoinix','Thersites','Laertes','Antilochos']

def request(path,body=None,token=None):
 headers={'Content-Type':'application/json'}
 if token:headers['Authorization']='Bearer '+token
 req=urllib.request.Request(BASE+path,data=None if body is None else json.dumps(body).encode(),headers=headers)
 with urllib.request.urlopen(req,timeout=25) as r:return json.load(r)
def sql(text):
 p=subprocess.run(['docker','exec','-i','megaron-acc-postgres-1','psql','-U','poleia','-d','poleia','-XAtq','-v','ON_ERROR_STOP=1'],input=text,text=True,capture_output=True)
 if p.returncode:raise RuntimeError(p.stderr.strip())
 return p.stdout.strip()
def lit(value):return "'"+str(value).replace("'","''")+"'"
def rows(query):return json.loads(sql("SELECT coalesce(jsonb_agg(to_jsonb(x)),'[]'::jsonb) FROM ("+query+") x;"))
def check_rig():
 for container in ['megaron-acc-server-1','megaron-acc-postgres-1']:
  project=subprocess.check_output(['docker','inspect','-f','{{index .Config.Labels "com.docker.compose.project"}}',container],text=True).strip()
  assert project=='megaron-acc','Refusing a container outside the isolated acceptance project'
 h=request('/healthz');assert str(h['commit']).startswith('7f0cdeb') and int(h['migration'])==203,h
 env=subprocess.check_output(['docker','inspect','-f','{{range .Config.Env}}{{println .}}{{end}}','megaron-acc-server-1'],text=True)
 cadence=int(next(x.split('=',1)[1] for x in env.splitlines() if x.startswith('TICK_SECONDS=')))
 assert cadence>=3600,'Use ACC_TICK_SECONDS=3600 tools/acceptance.sh up so text fixtures remain stable'
 return h,cadence

def main():
 OUT.mkdir(parents=True,exist_ok=True);CFG.mkdir(parents=True,exist_ok=True);CFG.chmod(0o700)
 health,cadence=check_rig()
 world=rows("SELECT id,current_tick FROM worlds WHERE status='active' ORDER BY created_at DESC LIMIT 1")[0]
 wid=str(uuid.UUID(world['id']))
 players={}
 for name in NAMES:
  try:auth=request('/api/v1/auth/register',{'username':name,'email':name.lower()+'@texttest.acceptance.local','password':'acceptance-pw-123'})
  except urllib.error.HTTPError as e:
   if e.code!=409:raise
   auth=request('/api/v1/auth/login',{'username_or_email':name,'password':'acceptance-pw-123'})
  me=request('/api/v1/auth/me',token=auth['access_token'])
  players[name]={'id':str(uuid.UUID(me['id'])),'token':auth['access_token'],'cities':[]}
 # Only these dedicated fixture owners are rewritten. Other acceptance accounts
 # and all prod/live databases remain outside the script's write boundary.
 statements=['BEGIN;']
 for name,p in players.items():
  pid=lit(p['id']);w=lit(wid)
  statements += [f"UPDATE player_world_records SET settlement_id=NULL,last_settlement_id=NULL WHERE player_id={pid} AND world_id={w};",
   f"DELETE FROM founder_phase WHERE owner_id={pid} AND world_id={w};",
   f"DELETE FROM units WHERE owner_id={pid} AND world_id={w};",
   f"DELETE FROM settlement_placement WHERE settlement_id IN(SELECT id FROM settlements WHERE owner_id={pid} AND world_id={w});",
   f"DELETE FROM build_queue WHERE settlement_id IN(SELECT id FROM settlements WHERE owner_id={pid} AND world_id={w});",
   f"DELETE FROM buildings WHERE settlement_id IN(SELECT id FROM settlements WHERE owner_id={pid} AND world_id={w});",
   f"DELETE FROM kharis_notice_state WHERE player_id={pid} AND world_id={w};",
   f"DELETE FROM notifications WHERE player_id={pid} AND world_id={w};",
   f"INSERT INTO player_world_records(player_id,world_id,kharis_amount,kharis_rate,kharis_cap,kharis_calc_tick) VALUES({pid},{w},40,0,100,current_world_tick()) ON CONFLICT(player_id,world_id) DO UPDATE SET status='active',kharis_amount=40,kharis_rate=0,kharis_cap=100,kharis_calc_tick=current_world_tick();"]
  count=2 if name=='Phoinix' else 1
  existing=rows(f"SELECT id,province_id FROM settlements WHERE owner_id={pid} AND world_id={w} ORDER BY is_capital DESC,name")
  assert len(existing) in (0,count),'Unexpected non-fixture city count for '+name
  for n in range(count):
   province=existing[n]['province_id'] if existing else str(uuid.uuid4());city=existing[n]['id'] if existing else str(uuid.uuid4());p['cities'].append({'id':city,'province':province})
   population=11000 if name=='Phoinix' else 1000
   statements += [f"INSERT INTO provinces(id,world_id,map_q,map_r,terrain_type) SELECT {lit(province)},{w},q,r,terrain FROM map_tiles mt WHERE world_id={w} AND terrain NOT IN ('sea','deep_sea','mountain_red') AND NOT EXISTS(SELECT 1 FROM provinces pp WHERE pp.world_id=mt.world_id AND pp.map_q=mt.q AND pp.map_r=mt.r) ORDER BY q,r LIMIT 1 ON CONFLICT(id) DO NOTHING;",
    f"INSERT INTO settlements(id,world_id,province_id,name,culture_id,owner_id,control_type,is_capital,population,state) VALUES({lit(city)},{w},{lit(province)},{lit(name+' '+str(n+1))},'minoan',{pid},{lit('capital' if n==0 else 'colony')},{'true' if n==0 else 'false'},{population},'active') ON CONFLICT(id) DO UPDATE SET population=EXCLUDED.population,state='active';",
    f"INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) SELECT {lit(city)},key,1000,0,100000,current_world_tick() FROM goods ON CONFLICT(settlement_id,good_key) DO UPDATE SET amount=1000,rate=0,cap=100000,calc_tick=current_world_tick();"]
   if n==0:
    level=2 if name=='Antilochos' else 1
    temple=str(uuid.uuid4());p['temple']=temple
    statements += [f"INSERT INTO buildings(id,settlement_id,building_type,level,god_id) VALUES({lit(temple)},{lit(city)},'temple',{level},'potnia');",
     f"UPDATE player_world_records SET settlement_id={lit(city)} WHERE player_id={pid} AND world_id={w};"]
    if name=='Eumelos':statements.append(f"INSERT INTO settlement_placement(settlement_id,gubbe_ordinal,target_kind,building_type,good_key,building_id) VALUES({lit(city)},1,'building','temple','cult',{lit(temple)});")
  initial=32 if name=='Phoinix' else 80 if name=='Laertes' else 45 if name=='Antilochos' else 40
  statements.append(f"UPDATE player_world_records SET kharis_amount={initial} WHERE player_id={pid} AND world_id={w};")
 statements.append('COMMIT;');sql('\n'.join(statements))
 # Execute the real recurring handler through the running server's event worker.
 event=sql(f"INSERT INTO scheduled_events(world_id,event_type,payload,process_after,due_tick,priority) VALUES({lit(wid)},'KharisTick','{{}}',now(),current_world_tick(),60) RETURNING id;")
 event=int(event)
 for _ in range(45):
  state=rows(f"SELECT processed_at,failed_at FROM scheduled_events WHERE id={event}")[0]
  if state['failed_at']:raise RuntimeError('Acceptance KharisTick failed; inspect rig logs')
  if state['processed_at']:break
  time.sleep(1)
 else:raise RuntimeError('Acceptance event worker did not complete the KharisTick within45s')
 ph=players['Phoinix'];notice=rows(f"SELECT body_json FROM notifications WHERE player_id={lit(ph['id'])} AND world_id={lit(wid)} AND kind='KharisDeclining'")
 assert len(notice)==1,notice
 # Preserve exact requested reading states after that one actual world tick.
 reset=['BEGIN;']
 for name,value in [('Eumelos',40),('Laertes',80),('Antilochos',45),('Thersites',12)]:
  reset.append(f"UPDATE player_world_records SET kharis_amount={value},kharis_rate=0,kharis_calc_tick=current_world_tick() WHERE player_id={lit(players[name]['id'])} AND world_id={lit(wid)};")
 # Explicit permitted fallback: consistent completed harvest-loss fixture,
 # persisted notice+seven-tick shield and a real reduction of seeded grain.
 th=players['Thersites'];sid=th['cities'][0]['id'];pid=th['id'];province=th['cities'][0]['province']
 reset += [f"UPDATE settlement_goods SET amount=200,rate=0,calc_tick=current_world_tick() WHERE settlement_id={lit(sid)} AND good_key='grain';",
  f"UPDATE settlement_goods SET amount=amount*0.5 WHERE settlement_id={lit(sid)} AND good_key='grain';",
  f"INSERT INTO kharis_notice_state(world_id,player_id,scope_id,condition,state,observed_tick,context) VALUES({lit(wid)},{lit(pid)},{lit(pid)},'divine_punishment','protected',current_world_tick(),jsonb_build_object('until_tick',current_world_tick()+7)) ON CONFLICT(world_id,player_id,scope_id,condition) DO UPDATE SET state='protected',observed_tick=EXCLUDED.observed_tick,context=EXCLUDED.context;",
  f"INSERT INTO notifications(world_id,player_id,kind,level,body_json,world_tick) SELECT {lit(wid)},{lit(pid)},'DivinePunishment',2,jsonb_build_object('type','harvest_failure','amount',100,'name','Thersites 1','settlement_id',{lit(sid)},'province_id',{lit(province)},'protection_until_tick',current_tick+7,'protection_until',last_tick_at+7*make_interval(secs=>{cadence})),current_tick FROM worlds WHERE id={lit(wid)};",'COMMIT;']
 sql('\n'.join(reset))
 BIN.parent.mkdir(parents=True,exist_ok=True)
 subprocess.run(['go','build','-o',str(BIN),'./cmd/keryx'],cwd=ROOT/'server',check=True)
 commands={'Eumelos':['rite','--list'],'Phoinix':['notifications','--kind','KharisDeclining'],'Thersites':['status'],'Laertes':['status'],'Antilochos':['demolish','--type','temple','--god','potnia']}
 transcript=[]
 for name,p in players.items():
  config=CFG/(name.lower()+'.json')
  config.write_text(json.dumps({'server':BASE,'token':p['token'],'world_id':wid,'player_id':p['id'],'province_id':p['cities'][0]['province'],'username':name},indent=2)+'\n');config.chmod(0o600)
  env={'HOME':str(Path.home()),'PATH':os.environ.get('PATH','/usr/bin:/bin'),'POLEIA_CONFIG':str(config)}
  result=subprocess.run([str(BIN),*commands[name]],env=env,input="",text=True,capture_output=True)
  expected=0
  assert result.returncode==expected,(name,result.stdout,result.stderr)
  text=result.stdout+result.stderr
  (OUT/(name.lower()+'.txt')).write_text(text)
  transcript.append('## '+name+'\n\n```text\n'+text.rstrip()+'\n```\n')
  # Tokens stay only in mode0600 configs, never in committed proof artefacts.
  p.pop('token');p['config']=str(config);p['command']=' '.join(commands[name])
 snapshots=rows(f"SELECT p.username,r.kharis_amount,r.kharis_cap,s.id AS city_id,s.population,(SELECT max(b.level) FROM buildings b JOIN settlements ss ON ss.id=b.settlement_id WHERE ss.owner_id=p.id AND ss.world_id=r.world_id AND b.building_type='temple') AS temple_level,(SELECT count(*) FROM settlement_placement sp WHERE sp.settlement_id=s.id AND sp.good_key='cult') AS staffed FROM player_world_records r JOIN players p ON p.id=r.player_id JOIN settlements s ON s.id=r.settlement_id WHERE r.world_id={lit(wid)} AND p.id IN ({','.join(lit(p['id']) for p in players.values())}) ORDER BY p.username")
 assert len(snapshots)==5
 expected={'Eumelos':(40,1,1),'Phoinix':(29.5,1,0),'Thersites':(12,1,0),'Laertes':(80,1,0),'Antilochos':(45,2,0)}
 for snap in snapshots:
  value,level,staffed=expected[snap['username']]
  assert abs(snap['kharis_amount']-value)<1e-8 and snap['temple_level']==level and snap['staffed']==staffed,snap
 shield=rows(f"SELECT (context->>'until_tick')::int-current_world_tick() AS remaining FROM kharis_notice_state WHERE player_id={lit(th['id'])} AND world_id={lit(wid)} AND condition='divine_punishment'")[0]
 assert shield['remaining']==7,shield
 assert next(x for x in snapshots if x['username']=='Antilochos')['kharis_amount']==45
 assert sql(f"SELECT count(*) FROM buildings WHERE id={lit(players['Antilochos']['temple'])};")=='1'
 manifest={'healthz':health,'world':wid,'tick_seconds':cadence,'real_kharis_event':event,'thersites_source':'consistent notice-state/notification fixture (allowed fallback), not a sampled punishment','players':players,'snapshots':snapshots}
 (OUT/'manifest.json').write_text(json.dumps(manifest,indent=2)+'\n')
 provenance=subprocess.check_output([str(ROOT/'tools/acceptance.sh'),'provenance'],text=True,cwd=ROOT).strip()
 (OUT/'SCENARIOS.md').write_text('# Kharis TEXT-test — ORDER29\n\n'+provenance+'\n\nKeryx: `'+str(BIN)+'`. Configs: `'+str(CFG)+'/<namn>.json` (mode0600).\n\nPhoinix: faktisk worker-hanterad KharisTick, id '+str(event)+'. Thersites: tillåten konsistent state/notis-fixtur. Syntetiska acceptansstäder; ORDER29-klienten har nya texter, serverreglerna är oförändrade. Riggen lämnas igång.\n\n'+'\n'.join(transcript))
 print('Ready: five configs, five transcripts, real KharisTick '+str(event)+'. Rig remains running at '+BASE)
if __name__=='__main__':main()
