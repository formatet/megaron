#!/usr/bin/env python3
"""Fresh PG16: upgrade/rollback changes exactly the hills grain catalogue row."""
from pathlib import Path
import json,os,secrets,subprocess,time,math
ROOT=Path(__file__).resolve().parents[3];OUT=Path(__file__).resolve().parent
env={k:os.environ[k] for k in ('HOME','PATH')};name='inland-catalogue-'+secrets.token_hex(4)
def call(args,**kw):return subprocess.check_output(args,text=True,env=env,**kw).strip()
try:
 call(['docker','run','-d','--rm','--name',name,'-e','POSTGRES_PASSWORD=pw','-p','127.0.0.1::5432','postgres:16-alpine'])
 port=json.loads(call(['docker','inspect',name]))[0]['NetworkSettings']['Ports']['5432/tcp'][0]['HostPort']
 for _ in range(100):
  if subprocess.run(['docker','exec',name,'pg_isready','-h','127.0.0.1','-U','postgres','-q'],capture_output=True).returncode==0:break
  time.sleep(.1)
 dsn=f'postgres://postgres:pw@127.0.0.1:{port}/postgres?sslmode=disable'
 def migrate(target):call(['migrate','-path','db/migrations','-database',dsn,'goto',str(target)],cwd=ROOT/'server')
 def snapshot():
  sql="SELECT json_build_object('migration',(SELECT version FROM schema_migrations),'rules',(SELECT json_agg(pr ORDER BY id) FROM production_rules pr),'goods',(SELECT json_agg(g ORDER BY key) FROM goods g))"
  return json.loads(call(['docker','exec',name,'psql','-U','postgres','-Atc',sql]))
 migrate(181);before=snapshot();migrate(182);after=snapshot()
 assert before['goods']==after['goods'],'goods valuation or catalogue changed'
 diffs=[(a,b) for a,b in zip(before['rules'],after['rules']) if a!=b]
 assert len(before['rules'])==len(after['rules']) and len(diffs)==1,diffs
 a,b=diffs[0];assert a['terrain_type']=='hills' and a['good_key']=='grain' and a['building_type'] is None
 assert math.isclose(a['rate_per_tick'],1/3) and b['rate_per_tick']==.5
 assert {k:v for k,v in a.items() if k!='rate_per_tick'}=={k:v for k,v in b.items() if k!='rate_per_tick'}
 migrate(181);rolled=snapshot()
 assert rolled['goods']==before['goods']
 for a,b in zip(before['rules'],rolled['rules']):
  assert {k:v for k,v in a.items() if k!='rate_per_tick'}=={k:v for k,v in b.items() if k!='rate_per_tick'}
  assert math.isclose(a['rate_per_tick'],b['rate_per_tick'],rel_tol=1e-14)
 (OUT/'catalogue.json').write_text(json.dumps({'before_migration':181,'after_migration':182,'changed_rows':diffs,'all_other_rules_identical':True,'all_goods_including_labor_value_identical':True,'rollback_green':True},indent=2)+'\n')
 print('fresh PG16 181 ->182 ->181: exactly ONE rate, no goods or other rules changed; rollback GREEN')
finally:subprocess.run(['docker','rm','-fv',name],stdout=subprocess.DEVNULL)
