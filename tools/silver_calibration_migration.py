#!/usr/bin/env python3
"""Populated 176 -> 177 -> 176 -> 177, with conserved balances/history."""
import json, pathlib, secrets, subprocess, time
ROOT=pathlib.Path(__file__).resolve().parents[1];OUT=ROOT/'docs/reviews/silverkalibrering';OUT.mkdir(parents=True,exist_ok=True)
name='silver-migration-'+secrets.token_hex(4)
def run(*a):
 r=subprocess.run(a,text=True,capture_output=True)
 if r.returncode:raise RuntimeError(r.stderr)
 return r.stdout.strip()
try:
 run('docker','run','-d','--rm','--name',name,'-e','POSTGRES_PASSWORD=proof','-e','POSTGRES_DB=proof','-p','127.0.0.1::5432','postgres:16-alpine')
 for _ in range(120):
  if subprocess.run(['docker','exec',name,'pg_isready','-U','postgres','-q'],capture_output=True).returncode==0:break
  time.sleep(.25)
 time.sleep(1)
 port=json.loads(run('docker','inspect',name))[0]['NetworkSettings']['Ports']['5432/tcp'][0]['HostPort']
 dsn=f'postgres://postgres:proof@127.0.0.1:{port}/proof?sslmode=disable'
 def migrate(*a):return run('migrate','-path',str(ROOT/'server/db/migrations'),'-database',dsn,*a)
 def sql(s):return run('docker','exec',name,'psql','-U','postgres','-d','proof','-XAt','-v','ON_ERROR_STOP=1','-c',s)
 migrate('goto','176')
 sql("""INSERT INTO players(id,username,password_hash) VALUES('00000000-0000-0000-0000-000000000001','silver-migration-proof','unused');
 INSERT INTO worlds(id,name,status,current_tick) VALUES('00000000-0000-0000-0000-000000000002','silver-migration-proof','active',10);
 INSERT INTO provinces(id,world_id,map_q,map_r,coastal) VALUES
 ('00000000-0000-0000-0000-000000000003','00000000-0000-0000-0000-000000000002',0,0,false),
 ('00000000-0000-0000-0000-000000000004','00000000-0000-0000-0000-000000000002',10,0,true);
 INSERT INTO settlements(id,world_id,province_id,name,culture_id,owner_id,is_capital) VALUES
 ('00000000-0000-0000-0000-000000000005','00000000-0000-0000-0000-000000000002','00000000-0000-0000-0000-000000000003','Capital','minoan','00000000-0000-0000-0000-000000000001',true),
 ('00000000-0000-0000-0000-000000000006','00000000-0000-0000-0000-000000000002','00000000-0000-0000-0000-000000000004','Colony','minoan','00000000-0000-0000-0000-000000000001',false);
 INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) VALUES
 ('00000000-0000-0000-0000-000000000005','silver',100,2.304,6480,8),
 ('00000000-0000-0000-0000-000000000006','silver',5000,0,6000,10);
 INSERT INTO player_world_records(player_id,world_id,status,last_settlement_id) VALUES
 ('00000000-0000-0000-0000-000000000001','00000000-0000-0000-0000-000000000002','active','00000000-0000-0000-0000-000000000005');
 INSERT INTO founder_phase(world_id,owner_id,population,grain_amount,grain_rate,silver_amount,silver_rate,calc_tick)
 VALUES('00000000-0000-0000-0000-000000000002','00000000-0000-0000-0000-000000000001',400,1200,0,60,-1,10);
 INSERT INTO divine_valuations(world_id,good_key,divine_value) VALUES('00000000-0000-0000-0000-000000000002','grain',99999);
 INSERT INTO events(world_id,stream_id,stream_type,event_type,payload) VALUES
 ('00000000-0000-0000-0000-000000000002','00000000-0000-0000-0000-000000000005','religion','RiteCast','{"offering_worth":200,"offering_baseline":200}'),
 ('00000000-0000-0000-0000-000000000002','00000000-0000-0000-0000-000000000005','province','KharisOffering','{"scarcity_factor":2,"kharis_gain":2.4}');""")
 history="SELECT json_agg(payload ORDER BY id)::text FROM events"
 oldhistory=sql(history)
 oldprices=sql("SELECT json_agg(json_build_array(key,base_value) ORDER BY key)::text FROM goods")
 before=json.loads(sql("SELECT json_agg(json_build_array(settlement_id,settled(amount,rate,calc_tick)) ORDER BY settlement_id)::text FROM settlement_goods WHERE good_key='silver'"))
 migrate('up')
 assert sql("SELECT version||'|'||dirty FROM schema_migrations")=='177|false'
 after=json.loads(sql("SELECT json_agg(json_build_array(settlement_id,amount,rate,cap) ORDER BY settlement_id)::text FROM settlement_goods WHERE good_key='silver'"))
 assert abs(after[0][1]-104.608)<1e-9 and after[0][2]==1 and after[0][3]==1460,after
 assert after[1][1]==5000 and after[1][2]==0 and after[1][3]>=5000,after
 assert sql('SELECT is_refugee FROM founder_phase')=='t'
 assert sql('SELECT COUNT(*) FROM divine_labor_valuations')=='0'
 assert sql('SELECT divine_value FROM divine_valuations')=='99999'
 assert sql(history)==oldhistory
 assert sql("SELECT json_agg(json_build_array(key,base_value) ORDER BY key)::text FROM goods")==oldprices
 first=after
 migrate('down','1');assert sql('SELECT version FROM schema_migrations')=='176'
 assert sql(history)==oldhistory
 migrate('up');assert sql("SELECT version||'|'||dirty FROM schema_migrations")=='177|false'
 after=json.loads(sql("SELECT json_agg(json_build_array(settlement_id,amount,rate,cap) ORDER BY settlement_id)::text FROM settlement_goods WHERE good_key='silver'"))
 assert after==first,(after,first)
 (OUT/'migration-verification.json').write_text(json.dumps({'before_settled_balances':before,'after_amount_rate_cap':after,'legacy_events_unchanged':True,'legacy_prices_unchanged':True,'legacy_index_not_copied':True,'refugee_backfill':True,'down_up_conserves_silver':True,'migration':177,'dirty':False},indent=2)+'\n')
 print('PASS populated migration176->177, down/up, balance/cap/rate/refugee/history/index preservation')
finally:subprocess.run(['docker','rm','-fv',name],capture_output=True)
