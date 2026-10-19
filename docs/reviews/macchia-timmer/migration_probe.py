#!/usr/bin/env python3
"""Populated188→189→188→189; repeated up and actual recompute-all, own fresh PG."""
from pathlib import Path
import json,os,secrets,subprocess,tempfile,time,uuid
HERE=Path(__file__).resolve().parent;ROOT=HERE.parents[2]
name='macchia22-migration-'+secrets.token_hex(4)
clean={k:os.environ[k] for k in ['HOME','PATH']}
def run(*args,**kw):return subprocess.check_output(args,text=True,**kw).strip()
run('docker','run','-d','--rm','--name',name,'-p','127.0.0.1::5432','-e','POSTGRES_PASSWORD=local-proof-only','postgres:16-alpine')
try:
 bindings=json.loads(run('docker','inspect',name))[0]['NetworkSettings']['Ports'];port=int(bindings['5432/tcp'][0]['HostPort'])
 dsn=f'postgres://postgres:local-proof-only@127.0.0.1:{port}/postgres?sslmode=disable'
 for _ in range(60):
  if subprocess.run(['docker','exec',name,'pg_isready','-h','127.0.0.1','-U','postgres','-q'],capture_output=True).returncode==0:break
  time.sleep(.25)
 def migrate(*args):
  try:return run('migrate','-path',str(ROOT/'server/db/migrations'),'-database',dsn,*args,stderr=subprocess.STDOUT)
  except subprocess.CalledProcessError as e:raise RuntimeError(e.output) from None
 def sql(text):return run('docker','exec','-i',name,'psql','-X','-U','postgres','-Atq','-v','ON_ERROR_STOP=1',input=text)
 migrate('goto','188')
 w,p,o,s=[str(uuid.uuid4()) for _ in range(4)]
 sql(f"""
 INSERT INTO worlds(id,name,status,current_tick)VALUES('{w}','migration continuity','active',100);
 INSERT INTO players(id,username,password_hash)VALUES('{o}','continuity','not-a-login');
 INSERT INTO provinces(id,world_id,map_q,map_r,terrain_type)VALUES('{p}','{w}',0,0,'plains');
 INSERT INTO settlements(id,world_id,province_id,owner_id,name,culture_id,control_type,is_capital,population)VALUES('{s}','{w}','{p}','{o}','Continuity','minoan','capital',true,1000);
 INSERT INTO map_tiles(world_id,q,r,terrain)VALUES('{w}',0,0,'plains'),('{w}',1,0,'forest_olive_grove'),('{w}',0,1,'scrub_maquis'),('{w}',-1,0,'hills'),('{w}',1,-1,'river_valley');
 INSERT INTO buildings(settlement_id,building_type,level,hex_q,hex_r)VALUES('{s}','lumbermill',1,1,0);
 INSERT INTO settlement_placement(settlement_id,gubbe_ordinal,target_kind,hex_q,hex_r,good_key)VALUES('{s}',1,'hex',1,0,'timber');
 INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick)VALUES('{s}','timber',37,1.7,1000000,100),('{s}','grain',99,0.5,1000000,100),('{s}','stone',12,0,1000000,100);
 """)
 def snapshot():
  fields=[]
  for table in ['production_rules','settlements','settlement_goods','buildings','settlement_placement']:
   order='id' if table!='settlement_placement' else 'gubbe_ordinal'
   fields.append("'"+table+"',(SELECT json_agg(x ORDER BY "+order+") FROM "+table+" x)")
  return json.loads(sql('SELECT json_build_object('+','.join(fields)+');'))
 before=snapshot();migrate('up','1');after=snapshot()
 for table in before:
  if table!='production_rules':assert before[table]==after[table],table+' changed during migration'
 old_ids={r['id']:r for r in before['production_rules']};new=[r for r in after['production_rules'] if r['id'] not in old_ids]
 assert {r['terrain_type'] for r in new}=={'scrub_maquis','hills','river_valley'} and len(new)==3
 assert all(r['rate_per_tick']==.33 and r['good_key']=='timber' and r['building_type'] is None for r in new)
 assert all(r==next(x for x in after['production_rules'] if x['id']==r['id']) for r in old_ids.values())
 sql((ROOT/'server/db/migrations/189_sparse_timber.up.sql').read_text());assert snapshot()==after,'repeated up duplicated or changed rows'
 migrate('down','1');assert snapshot()==before,'down did not restore exact original rule rows or city data'
 migrate('up','1')
 pre_recompute=snapshot()
 with tempfile.TemporaryDirectory(prefix='macchia22-recompute-') as temp:
  binary=Path(temp)/'recompute'
  subprocess.run(['go','build','-o',str(binary),'./cmd/recompute-all'],cwd=ROOT/'server',env=clean,check=True)
  recompute_env=clean|{'DATABASE_URL':dsn}
  dry=run(str(binary),'-dry-run',env=recompute_env);assert snapshot()==pre_recompute,'dry-run wrote data'
  first=run(str(binary),env=recompute_env);a=snapshot()
  second=run(str(binary),env=recompute_env);b=snapshot()
  assert a==b,'second recompute changed unchanged city state'
  assert a['settlement_placement']==before['settlement_placement'],'valid olive worker moved'
  amounts=lambda snap:{r['good_key']:r['amount'] for r in snap['settlement_goods']}
  assert all(amounts(a)[k]==v for k,v in amounts(before).items()),'existing stocks changed at frozen tick'
  timber=next(r for r in a['settlement_goods'] if r['good_key']=='timber')
  assert abs(timber['rate']-1.7)<1e-9,'existing olive lumbermill yield changed'
 proof=dict(populated_upgrade='188→189: exact city/stock/building/placement rows unchanged',new_rules=new,original_rules_unchanged=len(old_ids),repeated_up='no duplicate or changed rows',down='exact baseline snapshot restored',recompute_dry_run=dry,recompute_first=first,recompute_second=second,second_recompute='exact snapshot unchanged',preserved_stocks=amounts(before),preserved_olive_rate=timber['rate'],schema=sql('SELECT version,dirty FROM schema_migrations'))
 (HERE/'migration-proof.json').write_text(json.dumps(proof,indent=2)+'\n');print(json.dumps(proof))
finally:subprocess.run(['docker','rm','-f',name],capture_output=True)
