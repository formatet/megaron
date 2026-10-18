#!/usr/bin/env python3
"""Restore a PRIVATE pg_dump archive into disposable PG16; dry-run, hash all
public table rows before/after, then apply locally and verify the same deltas.
Never connects to production. Usage: snapshot_dryrun.py private.dump outdir
"""
from pathlib import Path
import hashlib,json,os,secrets,subprocess,sys,time
ROOT=Path(__file__).resolve().parents[3]
dump=Path(sys.argv[1]).resolve();out=Path(sys.argv[2]).resolve();out.mkdir(parents=True,exist_ok=True)
name='restfolk-snapshot-'+secrets.token_hex(5)
def run(args,**kw):return subprocess.run(args,check=True,capture_output=True,**kw)
def sql(query):return run(['docker','exec',name,'psql','-U','postgres','-d','snapshot','-Atc',query],text=True).stdout.strip()
try:
 run(['docker','run','-d','--rm','--name',name,'-p','127.0.0.1::5432','-e','POSTGRES_PASSWORD=private-local','-e','POSTGRES_DB=snapshot','postgres:16-alpine'])
 for _ in range(100):
  if subprocess.run(['docker','exec',name,'pg_isready','-U','postgres','-q'],capture_output=True).returncode==0:break
  time.sleep(.2)
 with dump.open('rb') as f:run(['docker','exec','-i',name,'pg_restore','-U','postgres','-d','snapshot','--no-owner','--exit-on-error'],stdin=f)
 port=json.loads(run(['docker','inspect',name],text=True).stdout)[0]['NetworkSettings']['Ports']['5432/tcp'][0]['HostPort']
 env={k:os.environ[k] for k in ('HOME','PATH')};env['DATABASE_URL']=f'postgres://postgres:private-local@127.0.0.1:{port}/snapshot?sslmode=disable'
 def fingerprint():
  result={}
  for table in sql("SELECT tablename FROM pg_tables WHERE schemaname='public' ORDER BY tablename").splitlines():
   # Table names are catalogue-owned; output contains hashes only, no secret rows.
   q='SELECT count(*),md5(COALESCE(string_agg(row::text,\'\' ORDER BY row::text),\'\')) FROM (SELECT to_jsonb(t) AS row FROM public."'+table+'" t) x'
   result[table]=sql(q)
  return result
 before=fingerprint()
 def execute(label,dry):
  r=run(['go','run','./cmd/recompute-all',*(['-dry-run'] if dry else [])],cwd=ROOT/'server',env=env,text=True);(out/(label+'.log')).write_text(r.stdout+r.stderr);return r.stdout
 first=execute('dryrun',True)
 after=fingerprint();assert after==before,'dryrun persisted changes'
 second=execute('apply-private-clone',False)
 # Per-city before/after lines identical; only final summary differs.
 assert first.split('\n\n')[0]==second.split('\n\n')[0],(first,second)
 third=execute('dryrun-after-apply',True);assert '0 changed, 0 failed' in third,third
 (out/'snapshot-proof.json').write_text(json.dumps({'dump_sha256':hashlib.sha256(dump.read_bytes()).hexdigest(),'migration':sql('SELECT version,dirty FROM schema_migrations'),'table_hashes_before':before,'table_hashes_after_dryrun':after,'all_public_tables_unchanged':True,'dryrun_equals_apply_deltas':True,'second_dryrun_no_change':True,'source':'read-only live pg_dump, high-volume heartbeat/tick claim table data excluded; no server started on clone'},indent=2))
 print(first,flush=True);print('private clone dry-run: all table hashes unchanged; apply deltas identical; repeat 0 changed',flush=True)
finally:subprocess.run(['docker','rm','-fv',name],capture_output=True)
