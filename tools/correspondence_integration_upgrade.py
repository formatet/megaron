#!/usr/bin/env python3
"""Backup live read-only, rehearse205->209->205->209 ONLY in owned offline PG15.
Personal database dump stays outside the repo; report only aggregate hashes.
Usage: python3 tools/correspondence_integration_upgrade.py OUTPUT_JSON
"""
import hashlib,json,os,secrets,subprocess,sys,time
from datetime import datetime
from pathlib import Path
ROOT=Path(__file__).resolve().parents[1]
OUT=Path(sys.argv[1]).resolve();OUT.parent.mkdir(parents=True,exist_ok=True)
stamp=datetime.now().astimezone().strftime('%Y%m%d-%H%M%S')
archive=Path.home()/'megaron-arkiv'/('correspondence-integration-'+stamp)
archive.mkdir(mode=0o700,parents=True,exist_ok=True)
remote='/root/megaron-backups/before-correspondence-'+stamp+'.dump'
local=archive/'before.dump'
def run(*args,**kwargs):return subprocess.check_output(args,text=True,**kwargs).strip()
print('BACKUP_START',flush=True)
run('ssh','root@10.0.1.92',f'umask 077; mkdir -p /root/megaron-backups; cd /tmp && runuser -u postgres -- pg_dump -Fc poleia > {remote} && test -s {remote}')
run('scp','root@10.0.1.92:'+remote,str(local));local.chmod(0o600)
name='megaron-correspondence-upgrade-'+secrets.token_hex(4)
password=secrets.token_urlsafe(25)
try:
 run('docker','run','-d','--rm','--name',name,'-p','127.0.0.1::5432','-e','POSTGRES_PASSWORD='+password,'-e','POSTGRES_DB=correspondence_upgrade','postgres:15-alpine')
 for _ in range(120):
  if subprocess.run(['docker','exec',name,'pg_isready','-h','127.0.0.1','-U','postgres','-d','correspondence_upgrade','-q'],capture_output=True).returncode==0:break
  time.sleep(.25)
 else:raise RuntimeError('Owned PG15 did not become ready')
 with local.open('rb') as source:
  subprocess.run(['docker','exec','-i',name,'pg_restore','-U','postgres','-d','correspondence_upgrade','--no-owner','--no-privileges'],stdin=source,check=True,stdout=subprocess.DEVNULL)
 port=json.loads(run('docker','inspect',name))[0]['NetworkSettings']['Ports']['5432/tcp'][0]['HostPort']
 dsn=f'postgres://postgres:{password}@127.0.0.1:{port}/correspondence_upgrade?sslmode=disable'
 def sql(query):return run('docker','exec',name,'psql','-U','postgres','-d','correspondence_upgrade','-At','-v','ON_ERROR_STOP=1','-c',query)
 def state():
  result={}
  for table in ('worlds','settlements','settlement_goods','units','messengers'):
   row="to_jsonb(t) - 'correspondence_created_xid' - 'correspondence_created_epoch'" if table=='messengers' else 'to_jsonb(t)'
   result[table]=json.loads(sql(f"SELECT json_build_object('count',count(*),'hash',md5(COALESCE(string_agg(({row})::text,'' ORDER BY ({row})::text),''))) FROM {table} t"))
  return result
 before=state();version=sql('SELECT version::text||\'|\'||dirty::text FROM schema_migrations')
 assert version=='205|false',version
 clean={k:os.environ[k] for k in ('HOME','PATH')}
 def migrate(*args):
  p=subprocess.run(['migrate','-path',str(ROOT/'server/db/migrations'),'-database',dsn,*args],env=clean,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True)
  if p.returncode:raise RuntimeError('Offline migration failed: '+p.stdout.replace(password,'[owned-test-password]'))
  return p.stdout
 print('OFFLINE_UP205_209',flush=True);up=migrate('up');after=state()
 assert before==after,{'before':before,'after':after}
 assert sql('SELECT version::text||\'|\'||dirty::text FROM schema_migrations')=='209|false'
 assert sql('SELECT count(*) FROM correspondence_cases')=='0','Invented legacy recipients'
 print('OFFLINE_DOWN209_205',flush=True);down=migrate('down','4');rolled=state()
 assert before==rolled
 assert sql('SELECT version::text||\'|\'||dirty::text FROM schema_migrations')=='205|false'
 print('OFFLINE_REUP205_209',flush=True);reup=migrate('up');assert before==state()
 report={'result':'PASS','postgres':sql('SHOW server_version'),'backup_remote':remote,'backup_local':str(local),'dump_bytes':local.stat().st_size,'dump_sha256':hashlib.sha256(local.read_bytes()).hexdigest(),'baseline':before,'after':after,'migration':'209|false','legacy_cases':0,'roundtrip':'205->209->205->209; all five table counts and data hashes identical','upgrade_log':up,'downgrade_log':down,'reupgrade_log':reup}
 OUT.write_text(json.dumps(report,indent=2));print('PASS',OUT,flush=True)
finally:subprocess.run(['docker','rm','-fv',name],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
