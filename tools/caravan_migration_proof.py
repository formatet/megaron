#!/usr/bin/env python3
"""Migration159→160→159→160 on a fresh private PostgreSQL16 database.
Preserves a preexisting naval transport's exact ETA/manifest; no live DB access.
"""
from pathlib import Path
import json
import secrets
import subprocess
import sys
import time

root = Path(__file__).resolve().parents[1]
out = Path(sys.argv[1]).resolve()
name = 'megaron-caravan-migration-' + secrets.token_hex(4)

def command(*args):
 return subprocess.check_output(args, text=True, stderr=subprocess.PIPE).strip()

try:
 command('docker','run','-d','--rm','--name',name,'-e','POSTGRES_PASSWORD=proof','-e','POSTGRES_DB=proof','-p','127.0.0.1::5432','postgres:16-alpine')
 for _ in range(120):
  if subprocess.run(['docker','exec',name,'pg_isready','-U','postgres','-q'],capture_output=True).returncode==0: break
  time.sleep(.25)
 time.sleep(1)
 port = json.loads(command('docker','inspect',name))[0]['NetworkSettings']['Ports']['5432/tcp'][0]['HostPort']
 dsn=f'postgres://postgres:proof@127.0.0.1:{port}/proof?sslmode=disable'
 def sql(text):
  return command('docker','exec',name,'psql','-U','postgres','-d','proof','-tAX','-v','ON_ERROR_STOP=1','-c',text)
 def migrate(*args):
  command('migrate','-path',str(root/'server/db/migrations'),'-database',dsn,*args)
 migrate('goto','159')
 sql("""INSERT INTO players(id,username,password_hash) VALUES('00000000-0000-0000-0000-000000000001','migration-proof','unused');
 INSERT INTO worlds(id,name,status) VALUES('00000000-0000-0000-0000-000000000002','migration-proof','active');
 INSERT INTO transports(id,world_id,owner_id,kind,category,origin_q,origin_r,dest_q,dest_r,departs_at,arrives_at,due_tick)
 VALUES('00000000-0000-0000-0000-000000000003','00000000-0000-0000-0000-000000000002','00000000-0000-0000-0000-000000000001','trade','naval',0,0,5,0,'2026-10-07T00:00:00Z','2026-10-07T05:00:00Z',99);
 INSERT INTO transport_goods(transport_id,good_key,quantity) VALUES('00000000-0000-0000-0000-000000000003','grain',17);""")
 projection="SELECT jsonb_build_object('departs_at',departs_at,'arrives_at',arrives_at,'due_tick',due_tick,'status',status,'manifest',(SELECT jsonb_agg(to_jsonb(g)) FROM transport_goods g WHERE g.transport_id=t.id))::text FROM transports t"
 before=sql(projection)
 migrate('up')
 assert sql('SELECT version||\'|\'||dirty FROM schema_migrations')=='160|false'
 assert sql('SELECT journey IS NULL AND departed_tick IS NULL FROM transports')=='t'
 assert sql(projection)==before
 refused=subprocess.run(['docker','exec',name,'psql','-U','postgres','-d','proof','-tAX','-v','ON_ERROR_STOP=1','-c',"UPDATE transports SET departed_tick=1"],capture_output=True,text=True)
 assert refused.returncode!=0 and 'transports_journey_departure' in refused.stderr
 migrate('down','1')
 assert sql('SELECT version FROM schema_migrations')=='159'
 assert sql(projection)==before
 migrate('up')
 assert sql(projection)==before
 out.parent.mkdir(parents=True,exist_ok=True)
 out.write_text(json.dumps({'migration_cycle':[159,160,159,160],'legacy_projection_preserved':json.loads(before),'paired_constraint_rejects_partial_route':True,'final_schema':sql('SELECT version||\'|\'||dirty FROM schema_migrations')},indent=2)+'\n')
 print('migration159 ->160 ->159 ->160, legacy projection unchanged, paired constraint enforced')
finally:
 subprocess.run(['docker','rm','-fv',name],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
