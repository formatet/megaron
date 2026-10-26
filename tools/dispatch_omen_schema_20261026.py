#!/usr/bin/env python3
"""Fresh PostgreSQL migration215 →214 →215 and legacy-row retention proof."""
from pathlib import Path
import subprocess,json,secrets,time,os
root=Path(__file__).resolve().parents[1];out=root/'docs/reviews/dispatchtext-omen/schema.log';name='megaron-omen-schema-'+secrets.token_hex(4)
def run(*args,**kw):return subprocess.check_output(args,text=True,**kw).strip()
try:
 run('docker','run','-d','--rm','--name',name,'-e','POSTGRES_PASSWORD=proof','-e','POSTGRES_DB=proof','-p','127.0.0.1::5432','postgres:16-alpine')
 for _ in range(100):
  if subprocess.run(['docker','exec',name,'pg_isready','-h','127.0.0.1','-U','postgres','-q'],capture_output=True).returncode==0:break
  time.sleep(.1)
 port=json.loads(run('docker','inspect',name))[0]['NetworkSettings']['Ports']['5432/tcp'][0]['HostPort'];dsn=f'postgres://postgres:proof@127.0.0.1:{port}/proof?sslmode=disable'
 def sql(s):return run('docker','exec',name,'psql','-U','postgres','-d','proof','-At','-v','ON_ERROR_STOP=1','-c',s)
 log=[]
 for cmd in [('up',),('down','1'),('up','1')]:
  if cmd==('down','1'):
   sql("WITH p AS (INSERT INTO players(username,password_hash) VALUES('schema-proof','x') RETURNING id),w AS (INSERT INTO worlds(name,status) VALUES('schema-proof','archived') RETURNING id) INSERT INTO gossip_events(world_id,recipient_id,source_region,category,text,fact_data,transport_owner_id) SELECT w.id,p.id,'Here','omen','historical outcome','{\"true\":true}',p.id FROM w CROSS JOIN p")
  result=subprocess.run(['migrate','-path',str(root/'server/db/migrations'),'-database',dsn,*cmd],cwd=root/'server',capture_output=True,text=True,env={k:os.environ[k] for k in ('HOME','PATH')});assert result.returncode==0,result.stdout+result.stderr
  state=sql('SELECT version,dirty FROM schema_migrations');log.append(str(cmd)+': '+state)
  want='214|f' if cmd==('down','1') else '215|f';assert state==want,state
  if cmd==('up','1'):
   retained=sql("SELECT text,fact_data IS NULL,transport_owner_id IS NULL FROM gossip_events WHERE text='historical outcome'");assert retained=='historical outcome|t|t',retained;log.append('retained row after rollback/upgrade: '+retained)
 out.write_text('\n'.join(log)+'\n');print('PASS schema215→214→215, clean versions and retained old row')
finally:subprocess.run(['docker','rm','-fv',name],capture_output=True)
