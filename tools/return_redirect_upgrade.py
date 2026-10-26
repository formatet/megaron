#!/usr/bin/env python3
"""Read-only live backup → private owned PG16 clone → 216→217→216→217 proof.
The dump stays in process memory; only hashes/counts and correction outcomes are
written. Does not change the live world or publish a backup/account data.
Usage: python3 tools/return_redirect_upgrade.py OUTPUT.json
"""
from pathlib import Path
import hashlib,json,secrets,subprocess,sys,time
ROOT=Path(__file__).resolve().parents[1]
REMOTE=r'''
import pathlib,shlex,subprocess,sys
vals={}
for line in pathlib.Path('/opt/poleia/.env').read_text().splitlines():
 line=line.strip()
 if line and not line.startswith('#') and '=' in line:
  k,v=line.split('=',1)
  if k.strip()=='DATABASE_URL':vals[k.strip()]=shlex.split(v)[0]
r=subprocess.run(['pg_dump',vals['DATABASE_URL'],'--format=custom','--no-owner','--no-privileges'],stdout=sys.stdout.buffer,stderr=subprocess.PIPE)
if r.returncode:raise SystemExit('read-only backup failed')
'''
def run(*args,**kw):return subprocess.check_output(args,text=True,**kw).strip()
backup=subprocess.run(['ssh','root@10.0.1.92','python3','-'],input=REMOTE.encode(),capture_output=True,check=True).stdout
name='megaron-return-upgrade-'+secrets.token_hex(4)
try:
 run('docker','run','-d','--rm','--name',name,'-p','127.0.0.1::5432','-e','POSTGRES_PASSWORD=proof','-e','POSTGRES_DB=proof','postgres:16-alpine')
 for _ in range(100):
  if subprocess.run(['docker','exec',name,'pg_isready','-U','postgres','-q'],capture_output=True).returncode==0:break
  time.sleep(.2)
 subprocess.run(['docker','exec','-i',name,'pg_restore','-U','postgres','-d','proof','--no-owner','--no-privileges'],input=backup,capture_output=True,check=True)
 port=json.loads(run('docker','inspect',name))[0]['NetworkSettings']['Ports']['5432/tcp'][0]['HostPort']
 dsn=f'postgres://postgres:proof@localhost:{port}/proof?sslmode=disable'
 def sql(q):return run('docker','exec',name,'psql','-U','postgres','-d','proof','-XAt','-v','ON_ERROR_STOP=1','-c',q)
 def stable():
  values={}
  for table in ['worlds','players','settlements','provinces','settlement_goods','player_scouted_tiles']:
   values[table]=sql(f"SELECT md5(COALESCE(jsonb_agg(to_jsonb(t) ORDER BY to_jsonb(t)::text)::text,'[]')) FROM {table} t")
  values['unit_physical_state']=sql("SELECT md5(COALESCE(jsonb_agg(to_jsonb(u)-'status'-'settlement_id'-'updated_at' ORDER BY id)::text,'[]')) FROM units u")
  return values
 def locations():return json.loads(sql("SELECT COALESCE(jsonb_object_agg(id,jsonb_build_object('status',status,'settlement_id',settlement_id)),'{}') FROM units"))
 assert sql("SELECT version::text||'|'||dirty::text FROM schema_migrations")=='216|false'
 before=stable();before_locations=locations();last_event=int(sql('SELECT COALESCE(max(id),0) FROM events'))
 old_events=sql(f"SELECT md5(COALESCE(jsonb_agg(to_jsonb(e) ORDER BY id)::text,'[]')) FROM events e WHERE id<={last_event}")
 expected=json.loads(sql("SELECT COALESCE(jsonb_agg(u.id),'[]') FROM units u JOIN settlements s ON s.id=u.settlement_id JOIN provinces p ON p.id=s.province_id WHERE u.category='land' AND u.status='garrison' AND u.q IS NOT NULL AND u.r IS NOT NULL AND (u.q,u.r) IS DISTINCT FROM(p.map_q,p.map_r)"))
 steps=[]
 for args,want in [(['up'],'217|false'),(['down','1'],'216|false'),(['up'],'217|false')]:
  run('migrate','-path',str(ROOT/'server/db/migrations'),'-database',dsn,*args)
  assert sql("SELECT version::text||'|'||dirty::text FROM schema_migrations")==want
  assert stable()==before,'migration changed physical/resources/account/world state'
  assert sql(f"SELECT md5(COALESCE(jsonb_agg(to_jsonb(e) ORDER BY id)::text,'[]')) FROM events e WHERE id<={last_event}")==old_events,'changed old events'
  after=locations()
  for uid,prior in before_locations.items():
   want_state={'status':'positioned','settlement_id':None} if uid in expected else prior
   assert after[uid]==want_state,(uid,prior,after[uid])
  outcomes=int(sql(f"SELECT count(*) FROM events WHERE id>{last_event} AND event_type='UnitGarrisonLocationCorrectedV1'"))
  assert outcomes==len(expected),(outcomes,expected)
  steps.append({'command':args,'schema':want,'corrections':outcomes,'stable_hashes':True,'old_events_unchanged':True})
 Path(sys.argv[1]).write_text(json.dumps({'source_dump_sha256':hashlib.sha256(backup).hexdigest(),'dump_bytes':len(backup),'corrected_units':expected,'unchanged_hashes':before,'steps':steps},indent=2)+'\n')
 print('PASS offline live-clone 216→217→216→217:',len(expected),'correction(s), no teleport/payor/resource changes, old events intact')
finally:subprocess.run(['docker','rm','-f',name],capture_output=True)
