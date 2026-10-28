#!/usr/bin/env python3
"""Exercise destructive create-world ONLY against a newly owned disposable DB."""
from pathlib import Path
import subprocess,tempfile,secrets,json,os,time,sys
R=Path(__file__).resolve().parents[1];O=Path(sys.argv[1]).resolve();O.mkdir(parents=True,exist_ok=True)
name='megaron-river-writer-'+secrets.token_hex(5)
def run(*a,**kw):return subprocess.check_output(a,text=True,**kw).strip()
clean={k:os.environ[k] for k in ['HOME','PATH']}
try:
 run('docker','run','-d','--rm','--name',name,'-e','POSTGRES_PASSWORD=proof','-e','POSTGRES_DB=proof','-p','127.0.0.1::5432','postgres:16-alpine')
 port=json.loads(run('docker','inspect',name))[0]['NetworkSettings']['Ports']['5432/tcp'][0]['HostPort']
 for _ in range(100):
  if subprocess.run(['docker','exec',name,'pg_isready','-U','postgres','-q'],capture_output=True).returncode==0:break
  time.sleep(.2)
 dsn=f'postgres://postgres:proof@localhost:{port}/proof?sslmode=disable'
 result=subprocess.run(['migrate','-path',str(R/'server/db/migrations'),'-database',dsn,'up'],text=True,capture_output=True);assert result.returncode==0,result.stderr
 with tempfile.TemporaryDirectory(prefix='river-writer-proof-') as td:
  binary=Path(td)/'create-world';run('go','build','-o',str(binary),'./cmd/create-world',cwd=R/'server',env=clean)
  result=subprocess.run([str(binary)],cwd=R/'server',env={**clean,'DATABASE_URL':dsn,'MAP_WIDTH':'40','MAP_HEIGHT':'30'},text=True,capture_output=True);(O/'create-world.log').write_text(result.stdout+result.stderr);assert result.returncode==0,result.stdout+result.stderr
  query="SELECT json_build_object('schema',(SELECT row_to_json(m) FROM schema_migrations m),'seed',(SELECT map_seed FROM worlds WHERE status='active'),'tiles',count(*),'rivers',count(*)FILTER(WHERE terrain IN('river','river_ford')),'missing',count(*)FILTER(WHERE terrain IN('river','river_ford') AND river_edges IS NULL),'known',count(*)FILTER(WHERE river_flow_known),'nonriver_metadata',count(*)FILTER(WHERE terrain NOT IN('river','river_ford') AND river_edges IS NOT NULL)) FROM map_tiles;"
  proof=json.loads(run('docker','exec',name,'psql','-U','postgres','-d','proof','-XAt','-c',query));assert proof['schema']=={'version':221,'dirty':False} and proof['tiles']==1200 and proof['rivers']>0 and proof['missing']==0 and proof['nonriver_metadata']==0,proof
  terrain_hash_query="SELECT md5(string_agg((to_jsonb(t)-'river_edges'-'river_upstream_edges'-'river_downstream_edges'-'river_flow_known')::text,'' ORDER BY q,r)) FROM map_tiles t"
  before_hash=run('docker','exec',name,'psql','-U','postgres','-d','proof','-XAt','-c',terrain_hash_query)
  run('migrate','-path',str(R/'server/db/migrations'),'-database',dsn,'down','1')
  assert run('docker','exec',name,'psql','-U','postgres','-d','proof','-XAt','-c','SELECT version FROM schema_migrations')=='220'
  run('migrate','-path',str(R/'server/db/migrations'),'-database',dsn,'up')
  with tempfile.TemporaryDirectory(prefix='.river-backfill-proof-',dir=R/'server') as gd:
   source=Path(gd)/'main.go';source.write_text('package main\nimport("context";"os";"formatet/megaron/server/internal/world";"github.com/jackc/pgx/v5/pgxpool")\nfunc main(){ctx:=context.Background();p,e:=pgxpool.New(ctx,os.Getenv("DATABASE_URL"));if e!=nil{panic(e)};defer p.Close();if e=world.CompleteRiverMetadata(ctx,p);e!=nil{panic(e)}}\n')
   run('go','run',str(source),cwd=R/'server',env={**clean,'DATABASE_URL':dsn})
  restored=json.loads(run('docker','exec',name,'psql','-U','postgres','-d','proof','-XAt','-c',query));assert restored['missing']==0 and restored['tiles']==proof['tiles'],restored
  assert before_hash==run('docker','exec',name,'psql','-U','postgres','-d','proof','-XAt','-c',terrain_hash_query)
  proof['schema_roundtrip']='221→220→221 + actual CompleteRiverMetadata';proof['static_map_hash']=before_hash;proof['roundtrip_preserves_map']=True
  proof['commit']=run('git','-C',str(R),'rev-parse','HEAD');proof['owned_disposable_database']=True;(O/'proof.json').write_text(json.dumps(proof,indent=2)+'\n');print('PASS create-world writes static river metadata:',json.dumps(proof))
finally:subprocess.run(['docker','rm','-fv',name],capture_output=True)
