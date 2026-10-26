#!/usr/bin/env python3
"""Read-only live dump to private PG16; prove route-memory repair and idempotency.
Backup stays in memory. Persist counts/hashes only. No live mutation.
Usage: python3 tools/route_sight_upgrade.py OUTPUT.json
"""
import hashlib,json,os,secrets,subprocess,sys,time
from pathlib import Path
ROOT=Path(__file__).resolve().parents[1]
REMOTE=r'''
import pathlib,shlex,subprocess,sys
for line in pathlib.Path('/opt/poleia/.env').read_text().splitlines():
 if line.startswith('DATABASE_URL='):
  dsn=shlex.split(line.split('=',1)[1])[0]
  r=subprocess.run(['pg_dump',dsn,'--format=custom','--no-owner','--no-privileges'],stdout=sys.stdout.buffer,stderr=subprocess.PIPE)
  if r.returncode:raise SystemExit('read-only backup failed')
  break
else:raise SystemExit('missing database configuration')
'''
def run(*args,**kw):return subprocess.check_output(args,text=True,**kw).strip()
backup=subprocess.run(['ssh','root@10.0.1.92','python3','-'],input=REMOTE.encode(),capture_output=True,check=True).stdout
name='megaron-route-sight-'+secrets.token_hex(4)
try:
 run('docker','run','-d','--rm','--name',name,'-p','127.0.0.1::5432','-e','POSTGRES_PASSWORD=proof','-e','POSTGRES_DB=proof','postgres:16-alpine')
 for _ in range(100):
  if subprocess.run(['docker','exec',name,'pg_isready','-h','127.0.0.1','-U','postgres','-q'],capture_output=True).returncode==0:break
  time.sleep(.2)
 subprocess.run(['docker','exec','-i',name,'pg_restore','-U','postgres','-d','proof','--no-owner','--no-privileges'],input=backup,capture_output=True,check=True)
 port=json.loads(run('docker','inspect',name))[0]['NetworkSettings']['Ports']['5432/tcp'][0]['HostPort']
 dsn=f'postgres://postgres:proof@localhost:{port}/proof?sslmode=disable'
 def sql(q):return run('docker','exec',name,'psql','-U','postgres','-d','proof','-XAt','-v','ON_ERROR_STOP=1','-c',q)
 world='8d1f34f1-96d8-4487-87dc-805948d86428';player='c3a51eae-557b-42c7-9631-7c9ae2edb4bf'
 def stable():
  tables=sql("SELECT tablename FROM pg_tables WHERE schemaname='public' AND tablename!='player_scouted_tiles' ORDER BY tablename").splitlines()
  return {t:sql(f"SELECT md5(COALESCE(jsonb_agg(to_jsonb(t) ORDER BY to_jsonb(t)::text)::text,'[]')) FROM {t} t") for t in tables}
 before=stable()
 def missing():return json.loads(sql(f"SELECT COALESCE(json_agg(json_build_array(q,r)),'[]'::json) FROM (VALUES(50,18),(49,17),(49,18),(49,19)) p(q,r) WHERE NOT EXISTS(SELECT 1 FROM player_scouted_tiles s WHERE s.world_id='{world}' AND s.player_id='{player}' AND s.q=p.q AND s.r=p.r)"))
 missing_before=missing()
 env={k:os.environ[k] for k in ['HOME','PATH','GOPATH','GOCACHE'] if k in os.environ};env['DATABASE_URL']=dsn
 command=['go','run','./cmd/backfill-route-sight','-world',world,'-player',player,'-witnesses',str(ROOT/'docs/reviews/rutthex-synminne/verified-land-eyes.json')]
 dry=json.loads(run(*command,cwd=ROOT/'server',env=env))
 assert missing()==missing_before and stable()==before,'dry-run changed data'
 first=json.loads(run(*command,'-apply',cwd=ROOT/'server',env=env))
 assert not missing(),'reported holes remain'
 second=json.loads(run(*command,'-apply',cwd=ROOT/'server',env=env))
 assert second['added']==0,'repair not idempotent'
 others=[]
 owners=json.loads(sql(f"SELECT COALESCE(json_agg(DISTINCT owner_id),'[]') FROM units WHERE world_id='{world}' AND march_route->>'version'='2' AND owner_id!='{player}'"))
 for owner in owners:
  cmd=['go','run','./cmd/backfill-route-sight','-world',world,'-player',owner]
  a=json.loads(run(*cmd,'-apply',cwd=ROOT/'server',env=env))
  b=json.loads(run(*cmd,'-apply',cwd=ROOT/'server',env=env));assert b['added']==0
  others.append({'first':a,'second':b})
 assert stable()==before,'repair changed data outside terrain memory'
 # Every pre-existing memory row survives; other players remain byte-identical.
 # Counts and hashes only leave the private clone.
 proof={'source_dump_sha256':hashlib.sha256(backup).hexdigest(),'missing_before':missing_before,'missing_after':missing(),'dry_run':dry,'first':first,'second':second,'other_owners':others,'unchanged_tables':before}
 Path(sys.argv[1]).write_text(json.dumps(proof,indent=2)+'\n')
 print('PASS private live clone: dry-run rollback, reported holes filled, idempotent,',len(before),'other tables unchanged')
finally:subprocess.run(['docker','rm','-f',name],capture_output=True)
