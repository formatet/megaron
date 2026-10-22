#!/usr/bin/env python3
"""Run the built server on the retained fresh-test DB and an isolated Redis. Never dev/live."""
from pathlib import Path
import json,os,socket,subprocess,sys,time,urllib.request,urllib.error,uuid
ROOT=Path(__file__).resolve().parents[3]
TMP=Path('/tmp/megaron-wants-proof');TMP.mkdir(exist_ok=True)
redis_name='codex-wants-proof-'+uuid.uuid4().hex[:10]
process=None
pg_container=sys.argv[1]
assert pg_container.startswith('megaron-gotest-') and pg_container.removeprefix('megaron-gotest-').isdigit()
database='wants_runtime_'+uuid.uuid4().hex[:12]
# Fresh schema in our retained container: Go tests can replace SQL helpers in their own DB.
subprocess.run(['docker','exec',pg_container,'psql','-U','postgres','-qc','CREATE DATABASE '+database],check=True)
db_url='postgres://postgres:pw@localhost:57900/'+database+'?sslmode=disable'
with (TMP/'migrations.log').open('w') as migration_log:
 subprocess.run(['migrate','-path','db/migrations','-database',db_url,'up'],cwd=ROOT/'server',stdout=migration_log,stderr=subprocess.STDOUT,check=True)
with socket.socket() as s:s.bind(('127.0.0.1',0));port=s.getsockname()[1]
base=f'http://127.0.0.1:{port}'
def http(path,method='GET',body=None,token=None):
 headers={}
 if body is not None:headers['Content-Type']='application/json';body=json.dumps(body).encode()
 if token:headers['Authorization']='Bearer '+token
 req=urllib.request.Request(base+path,data=body,headers=headers,method=method)
 try:
  with urllib.request.urlopen(req,timeout=5) as res:return res.status,res.read().decode()
 except urllib.error.HTTPError as res:return res.code,res.read().decode()
try:
 subprocess.run(['docker','run','-d','--rm','--name',redis_name,'-p','127.0.0.1::6379','redis:7-alpine'],check=True,stdout=subprocess.DEVNULL)
 redis_port=subprocess.check_output(['docker','port',redis_name,'6379/tcp'],text=True).strip().split(':')[-1]
 env={k:os.environ[k] for k in ('PATH','HOME') if k in os.environ}
 env.update(DATABASE_URL=db_url,REDIS_URL='127.0.0.1:'+redis_port,JWT_SECRET='isolated-wants-proof-only',PORT=str(port),WORLD_NAME='Wants retirement proof',MAP_WIDTH='30',MAP_HEIGHT='20',TICK_SECONDS='3600',CHRONICLE_DIR=str(TMP/'chronicles'),REPORTS_DIR=str(TMP/'reports'),POLEIA_REGISTRATION_CLOSED='0',STATIC_DIR=str(ROOT/'web/static'),TEMPLATE_DIR=str(ROOT/'web/templates'))
 with (TMP/'server.log').open('w') as log:
  process=subprocess.Popen([str(TMP/'server')],cwd=ROOT/'server',env=env,stdout=log,stderr=subprocess.STDOUT)
 deadline=time.monotonic()+60
 while True:
  if process.poll() is not None:raise RuntimeError((TMP/'server.log').read_text()[-4000:])
  try:
   code,body=http('/healthz')
   if code==200:break
  except (OSError,urllib.error.URLError):pass
  if time.monotonic()>deadline:raise TimeoutError((TMP/'server.log').read_text()[-4000:])
  time.sleep(.2)
 health=json.loads(body);assert health['migration']==198 and not health.get('migration_dirty',False),health
 code,body=http('/api/v1/worlds');assert code==200,(code,body)
 worlds=json.loads(body);worlds=worlds if isinstance(worlds,list) else worlds.get('worlds',[])
 assert worlds,body
 world=worlds[0]['id']
 code,body=http('/api/v1/auth/register','POST',{'username':'wants-proof-'+uuid.uuid4().hex[:8],'password':'local-proof-password'});assert code==201,(code,body)
 token=json.loads(body)['access_token']
 assert http('/api/v1/auth/me',token=token)[0]==200
 checks=[]
 for auth in (False,True):
  for method in ('GET','POST'):
   path=f'/api/v1/worlds/{world}/market/wants'
   code,body=http(path,method=method,token=token if auth else None)
   assert code in (404,405),(auth,method,code,body)
   checks.append({'authenticated':auth,'method':method,'status':code,'body':body.strip()})
 cli=subprocess.run([str(TMP/'keryx'),'wants'],cwd=TMP,env=env,capture_output=True,text=True)
 assert cli.returncode!=0 and 'unknown command "wants"' in cli.stderr,(cli.returncode,cli.stdout,cli.stderr)
 print(json.dumps({'healthz':health,'worlds_status':200,'authenticated_me_status':200,'market_wants':checks,'keryx':{'exit':cli.returncode,'stderr':cli.stderr.strip()}},indent=2))
finally:
 if process is not None:
  process.terminate()
  try:process.wait(timeout=10)
  except subprocess.TimeoutExpired:process.kill();process.wait()
 subprocess.run(['docker','rm','-f',redis_name],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
 subprocess.run(['docker','exec',pg_container,'psql','-U','postgres','-qc','DROP DATABASE '+database],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
