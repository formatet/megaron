#!/usr/bin/env python3
"""Populated177->178->177->178; per-world lazy balances and old events survive."""
import json,pathlib,secrets,subprocess,time
HERE=pathlib.Path(__file__).resolve().parent;ROOT=HERE.parents[2]
name='spjut-migration-'+secrets.token_hex(4)
def run(*a):return subprocess.check_output(a,text=True).strip()
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
 migrate('goto','177')
 sql("""INSERT INTO players(id,username,password_hash) SELECT ('00000000-0000-0000-0000-00000000000'||n)::uuid,'p'||n,'x' FROM generate_series(1,3) n;
 INSERT INTO worlds(id,name,status,current_tick) VALUES
 ('00000000-0000-0000-0000-000000000011','active-proof','active',100),
 ('00000000-0000-0000-0000-000000000012','archived-proof','archived',20);
 INSERT INTO founder_phase(world_id,owner_id,population,grain_amount,grain_rate,silver_amount,silver_rate,calc_tick,active,is_refugee) VALUES
 ('00000000-0000-0000-0000-000000000011','00000000-0000-0000-0000-000000000001',1000,1200,-1,480,-2,97,true,false),
 ('00000000-0000-0000-0000-000000000012','00000000-0000-0000-0000-000000000002',400,1200,0,60,-1,17,true,true),
 ('00000000-0000-0000-0000-000000000011','00000000-0000-0000-0000-000000000003',1000,1200,0,480,-2,97,false,false);
 INSERT INTO units(world_id,owner_id,type,category,size,status,unpaid_periods,carried_silver) VALUES
 ('00000000-0000-0000-0000-000000000011','00000000-0000-0000-0000-000000000001','spearman','land',100,'garrison',71,30),
 ('00000000-0000-0000-0000-000000000011','00000000-0000-0000-0000-000000000001','elite_infantry','land',100,'garrison',71,60);
 INSERT INTO events(world_id,stream_id,stream_type,event_type,payload) VALUES
 ('00000000-0000-0000-0000-000000000011','00000000-0000-0000-0000-000000000001','combat','UpkeepUnpaid','{"unit_type":"spearman","silver_unpaid":1,"reason":"silver_shortage"}'),
 ('00000000-0000-0000-0000-000000000011','00000000-0000-0000-0000-000000000001','religion','RiteCast','{"offering_value":100,"kharis":3}');""")
 history=sql('SELECT json_agg(payload ORDER BY id)::text FROM events')
 state=lambda:json.loads(sql('SELECT json_agg(json_build_array(owner_id,grain_amount,grain_rate,silver_amount,silver_rate,calc_tick,active) ORDER BY owner_id)::text FROM founder_phase'))
 units=lambda:json.loads(sql('SELECT json_agg(json_build_array(type,size,status,unpaid_periods,carried_silver) ORDER BY type)::text FROM units'))
 migrate('up');assert sql('SELECT version||\'|\'||dirty FROM schema_migrations')=='178|false'
 after=state();assert after[0][1:]==[1197,-1,474,0,100,True],after
 assert after[1][1:]==[1200,0,57,0,20,True],after
 assert after[2][1:]==[1200,0,480,-2,97,False],after
 u=units();assert u==[['elite_infantry',100,'garrison',71,60],['spearman',100,'garrison',0,30]],u
 assert sql('SELECT json_agg(payload ORDER BY id)::text FROM events')==history
 print('PASS178 uses eachworld tick100/20, preserves lazy474/57 and grain1197/1200; inactive unchanged; no manpower/carried/event changes; spear unpaid0, elite71')
 migrate('down','1');down=state();assert down[0][3]==474 and down[1][3]==57
 assert down[0][4]==-2 and down[1][4]==-1
 migrate('up');assert state()==after and units()==u
 assert sql('SELECT json_agg(payload ORDER BY id)::text FROM events')==history
 print('PASS down/up same ticks preserves all money and historic events; reset warnings not recreated')
 (HERE/'migration-proof.json').write_text(json.dumps({'after':after,'units':u,'historic_payloads':json.loads(history),'down_up_same':True},indent=2)+'\n')
finally:run('docker','rm','-f',name)
