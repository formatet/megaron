#!/usr/bin/env python3
"""Private read-only live dump -> disposable PG16 -> real migration184.
Public output contains only per-city food values, no auth or personal data.
"""
from pathlib import Path
import hashlib,json,os,secrets,subprocess,sys,time
ROOT=Path(__file__).resolve().parents[3];dump=Path(sys.argv[1]);out=Path(sys.argv[2]);name='one-food-'+secrets.token_hex(6)
def run(args,**kw):return subprocess.run(args,check=True,capture_output=True,**kw)
def sql(q):return run(['docker','exec',name,'psql','-v','ON_ERROR_STOP=1','-U','postgres','-d','snapshot','-Atc',q],text=True).stdout.strip()
try:
 run(['docker','run','-d','--rm','--name',name,'-p','127.0.0.1::5432','-e','POSTGRES_PASSWORD=private','-e','POSTGRES_DB=snapshot','postgres:16-alpine'])
 for _ in range(100):
  if subprocess.run(['docker','exec',name,'pg_isready','-U','postgres','-q'],capture_output=True).returncode==0:break
  time.sleep(.2)
 with dump.open('rb') as f:run(['docker','exec','-i',name,'pg_restore','-U','postgres','-d','snapshot','--no-owner','--exit-on-error'],stdin=f)
 query="""SELECT COALESCE(json_agg(x ORDER BY id,good_key),'[]') FROM (SELECT s.id,s.name,w.current_tick,g.good_key,
 GREATEST(0,COALESCE(sg.amount+sg.rate*GREATEST(0,w.current_tick-sg.calc_tick),0)) stock,
 COALESCE(g.amount,0) reserve,sg.rate,sg.cap,
 GREATEST(0,COALESCE(sg.amount+sg.rate*GREATEST(0,w.current_tick-sg.calc_tick),0))+COALESCE(g.amount,0) total
 FROM settlements s JOIN worlds w ON w.id=s.world_id CROSS JOIN (VALUES('grain'),('fish')) basket(k)
 LEFT JOIN settlement_goods sg ON sg.settlement_id=s.id AND sg.good_key=basket.k
 LEFT JOIN settlement_granary g ON g.settlement_id=s.id AND g.good_key=basket.k
 )x"""
 # Use basket key even when no reserve row exists.
 query=query.replace('g.good_key,','basket.k good_key,')
 before=json.loads(sql(query));silver_before=sql("SELECT md5(COALESCE(jsonb_agg(to_jsonb(g) ORDER BY settlement_id)::text,'')) FROM settlement_goods g WHERE good_key='silver'")
 events_before=sql("SELECT count(*) FROM events")
 migration=(ROOT/'server/db/migrations/184_remove_sitos.up.sql').read_text()
 sql('BEGIN;\n'+migration+'\nUPDATE schema_migrations SET version=184;\nCOMMIT;')
 after_query="""SELECT COALESCE(json_agg(x ORDER BY id,good_key),'[]') FROM (SELECT s.id,s.name,w.current_tick,sg.good_key,
 GREATEST(0,sg.amount+sg.rate*GREATEST(0,w.current_tick-sg.calc_tick)) stock,sg.rate,sg.cap
 FROM settlements s JOIN worlds w ON w.id=s.world_id JOIN settlement_goods sg ON sg.settlement_id=s.id
 WHERE sg.good_key IN ('grain','fish'))x"""
 after=json.loads(sql(after_query));index={(x['id'],x['good_key']):x for x in after}
 for b in before:
  a=index.get((b['id'],b['good_key']),{'stock':0})
  assert abs(a['stock']-b['total'])<1e-8,(b,a)
  if b['reserve']>0:assert a['cap']>=a['stock'],(b,a)
 silver_after=sql("SELECT md5(COALESCE(jsonb_agg(to_jsonb(g) ORDER BY settlement_id)::text,'')) FROM settlement_goods g WHERE good_key='silver'")
 assert silver_before==silver_after
 assert events_before==sql('SELECT count(*) FROM events')
 assert sql("SELECT to_regclass('public.settlement_granary') IS NULL AND to_regclass('public.processed_sitos_ticks') IS NULL")=='t'
 pending=sql("SELECT count(*) FROM scheduled_events WHERE event_type='SitosTick' AND processed_at IS NULL");assert pending=='0'
 port=json.loads(run(['docker','inspect',name],text=True).stdout)[0]['NetworkSettings']['Ports']['5432/tcp'][0]['HostPort']
 env={k:os.environ[k] for k in ('HOME','PATH')};env['DATABASE_URL']=f'postgres://postgres:private@127.0.0.1:{port}/snapshot?sslmode=disable'
 recompute=run(['go','run','./cmd/recompute-all','-world','f7c90afa-4653-46a6-b14c-473a268efa3e'],cwd=ROOT/'server',env=env,text=True)
 out.with_suffix('.recompute.log').write_text(recompute.stdout+recompute.stderr)
 after_recompute=json.loads(sql(after_query));recomputed={(x['id'],x['good_key']):x for x in after_recompute}
 for a in after:
  b=recomputed[(a['id'],a['good_key'])]
  assert abs(a['stock']-b['stock'])<1e-8,(a,b)
 result={'dump_sha256':hashlib.sha256(dump.read_bytes()).hexdigest(),'schema':sql('SELECT version,dirty FROM schema_migrations'),'before':before,'after':after,'after_recompute_all':after_recompute,'recompute_preserves_every_city_good_total':True,'all_city_good_totals_conserved':True,'silver_projection_unchanged':True,'historic_event_count_unchanged':True,'pending_retired_ticks':0,'limitations':'Private clone only. No server/tick workers running. Per-world lazy evaluation, equivalent to settled() for the active world. No live writes.'}
 out.write_text(json.dumps(result,indent=2))
 print('migration184 conservation GREEN',len(before),'city/good rows; silver and event count unchanged')
 for b in before:
  if b['reserve']>0:print(b['name'],b['good_key'],round(b['stock'],8),'+',round(b['reserve'],8),'=',round(index[b['id'],b['good_key']]['stock'],8))
finally:subprocess.run(['docker','rm','-fv',name],capture_output=True)
