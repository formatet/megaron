#!/usr/bin/env python3
"""Real GenerateMap + actual pickSpawnTile SQL, isolated copies and temporary PG.
Never connects to dev. Requires Go and Docker with postgres:16-alpine.
"""
import hashlib,json,os,pathlib,re,secrets,shutil,subprocess,tempfile,time
HERE=pathlib.Path(__file__).resolve().parent
ROOT=HERE.parents[2]
OUT=pathlib.Path(os.environ.get('SILVER_STUDY_OUT',str(HERE/'results')))
OUT.mkdir(parents=True,exist_ok=True)
def run(*args,**kw):return subprocess.check_output(args,text=True,**kw).strip()
source=(ROOT/'server/api/handlers/join.go').read_text()
query=re.search(r'`(WITH hosts AS \(.*?LIMIT 1)`',source,re.S).group(1)
(OUT/'spawn.sql').write_text(query+'\n')
manifest={'source_commit':run('git','-C',str(ROOT),'rev-parse','HEAD'),
 'mapgen_sha256':hashlib.sha256((ROOT/'server/internal/world/mapgen.go').read_bytes()).hexdigest(),
 'spawn_sql_sha256':hashlib.sha256(query.encode()).hexdigest(),
 'requested_seeds':[1009*i for i in range(1,21)],'width':64,'height':64,
 'method':'actual SQL, sequential active hosts, no moving/founding; seeded PostgreSQL RANDOM',
 'timber_terrains':['forest_olive_grove'],'variants':[4,5,10,20,40]}
name='megaron-silver-study-'+secrets.token_hex(4)
run('docker','run','-d','--rm','--name',name,'-e','POSTGRES_PASSWORD=isolated-study','-p','127.0.0.1::5432','postgres:16-alpine')
try:
 for _ in range(120):
  if subprocess.run(['docker','exec',name,'pg_isready','-U','postgres','-q'],capture_output=True).returncode==0:break
  time.sleep(.25)
 else:raise RuntimeError('temporary PG failed to start')
 port=json.loads(run('docker','inspect',name))[0]['NetworkSettings']['Ports']['5432/tcp'][0]['HostPort']
 env={**os.environ,'STUDY_DATABASE_URL':f'postgres://postgres:isolated-study@127.0.0.1:{port}/postgres?sslmode=disable'}
 with tempfile.TemporaryDirectory(prefix='silver-study-') as tmp:
  tmp=pathlib.Path(tmp)
  for f in ['go.mod','go.sum']:shutil.copy(ROOT/'server'/f,tmp/f)
  pkg=tmp/'internal/world';pkg.mkdir(parents=True)
  for f in (ROOT/'server/internal/world').glob('*.go'):
   if not f.name.endswith('_test.go'):shutil.copy(f,pkg/f.name)
  cmd=tmp/'cmd/probe';cmd.mkdir(parents=True);shutil.copy(HERE/'probe.go.txt',cmd/'main.go')
  original=(pkg/'mapgen.go').read_text()
  for target in manifest['variants']:
   # Experimental copies only; retained cluster size, spacing, rate and validation.
   changed,n=re.subn(r'(silverSourceFloor\s*=\s*)4',rf'\g<1>{target}',original)
   assert n==1
   (pkg/'mapgen.go').write_text(changed)
   subprocess.run(['go','build','-o',str(tmp/'probe'),'./cmd/probe'],cwd=tmp,check=True)
   with (OUT/f'maps-{target}.jsonl').open('w') as output,(OUT/f'maps-{target}.log').open('w') as log:
    subprocess.run([str(tmp/'probe'),str(OUT/'spawn.sql')],env=env,stdout=output,stderr=log,check=True)
   print(f'Completed {target} silver target',flush=True)
 (OUT/'manifest.json').write_text(json.dumps(manifest,indent=2)+'\n')
finally:run('docker','rm','-f',name)
subprocess.run(['python3',str(HERE/'summarize.py'),str(OUT)],check=True)
