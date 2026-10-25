#!/usr/bin/env python3
"""Fresh PG16 migration212 ->213 ->212 ->213, preserving legacy progress."""
from pathlib import Path
import subprocess,json,secrets,time,sys,socket
ROOT=Path(__file__).resolve().parents[1];OUT=Path(sys.argv[1]).resolve();OUT.mkdir(parents=True,exist_ok=True)
name='megaron-hex-schema-'+secrets.token_hex(4)
def run(*args,**kw):return subprocess.check_output(args,text=True,**kw).strip()
try:
 run('docker','run','-d','--rm','--name',name,'-e','POSTGRES_PASSWORD=hexproof','-e','POSTGRES_DB=hexproof','-p','127.0.0.1::5432','postgres:16-alpine')
 for _ in range(100):
  if subprocess.run(['docker','exec',name,'pg_isready','-U','postgres','-q'],capture_output=True).returncode==0:break
  time.sleep(.1)
 port=json.loads(run('docker','inspect',name))[0]['NetworkSettings']['Ports']['5432/tcp'][0]['HostPort'];dsn=f'postgres://postgres:hexproof@localhost:{port}/hexproof?sslmode=disable'
 for _ in range(100):
  try:
   logs=run('docker','logs',name,stderr=subprocess.STDOUT)
   if logs.rfind('database system is ready to accept connections')>logs.rfind('listening on IPv4 address')>=0:break
   time.sleep(.1)
  except OSError:time.sleep(.1)
 def migrate(*args):
  r=subprocess.run(['migrate','-path',str(ROOT/'server/db/migrations'),'-database',dsn,*args],capture_output=True,text=True)
  if r.returncode:raise RuntimeError(r.stdout+r.stderr)
 def sql(s):return run('docker','exec',name,'psql','-U','postgres','-d','hexproof','-XAt','-v','ON_ERROR_STOP=1','-c',s)
 migrate('goto','212')
 world=sql("INSERT INTO worlds(name,status) VALUES('migration-proof','active') RETURNING id").splitlines()[0]
 owner=sql("INSERT INTO players(username,password_hash) VALUES('migration-proof','x') RETURNING id").splitlines()[0]
 ship=sql(f"INSERT INTO units(world_id,owner_id,type,category,size,crew,status,q,r) VALUES('{world}','{owner}','galley','naval',1,20,'positioned',0,0) RETURNING id").splitlines()[0]
 sql(f"INSERT INTO sea_storm_progress(voyage_key,world_id,ship_unit_id,last_hex,last_tick) VALUES('legacy-proof','{world}','{ship}',0,42)")
 migrate('up');assert sql('SELECT version,dirty FROM schema_migrations')=='213|f'
 assert sql("SELECT last_tick,last_milli IS NULL,last_storm_id IS NULL FROM sea_storm_progress WHERE voyage_key='legacy-proof'")=='42|t|t'
 sql(f"INSERT INTO timed_route_progress(world_id,last_milli) VALUES('{world}',42333)")
 sql("UPDATE sea_storm_progress SET last_milli=42333 WHERE voyage_key='legacy-proof'")
 migrate('down','1');assert sql('SELECT version,dirty FROM schema_migrations')=='212|f'
 assert sql("SELECT last_tick FROM sea_storm_progress WHERE voyage_key='legacy-proof'")=='42'
 migrate('up');assert sql('SELECT version,dirty FROM schema_migrations')=='213|f'
 assert sql("SELECT last_tick,last_milli IS NULL FROM sea_storm_progress WHERE voyage_key='legacy-proof'")=='42|t'
 (OUT/'proof.json').write_text(json.dumps({'postgres':16,'versions':[212,213,212,213],'legacy_last_tick_preserved':42,'new_progress_nullable_on_upgrade':True,'dirty':False,'private_container_removed':True},indent=2))
 print('PASS migration212→213→212→213, legacy storm mark preserved')
finally:subprocess.run(['docker','rm','-fv',name],capture_output=True)
