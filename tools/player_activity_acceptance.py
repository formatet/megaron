#!/usr/bin/env python3
"""Authenticated activity: fresh PG/Redis/server, real HTTP/cookie/Keryx, offline schema cycle.
Usage: python3 tools/activity_bild_20261026.py OUTPUT
No shared services or credentials; no screenshots needed for this operational field.
"""
from pathlib import Path
import os,json,secrets,socket,subprocess,tempfile,time,urllib.request,urllib.error,hashlib,sys
ROOT=Path(__file__).resolve().parents[1]
OUT=Path(sys.argv[1]).resolve();OUT.mkdir(parents=True,exist_ok=True)
containers=[];proc=None
clean={k:os.environ[k] for k in ('HOME','PATH')};clean['CGO_ENABLED']='0'
def run(*args,**kw):return subprocess.check_output(args,text=True,**kw).strip()
def hashes():return {str(p.relative_to(ROOT)):hashlib.sha256(p.read_bytes()).hexdigest() for d in ('server','web') for p in sorted((ROOT/d).rglob('*')) if p.is_file() and p.suffix in ('.go','.js','.md','.html','.sql','.css','.json') and not p.name.endswith('_test.go')}
source_before=hashes()
(OUT/'source-before.json').write_text(json.dumps(source_before,indent=2))
def launch(kind,image,args):
 name='megaron-activity-'+secrets.token_hex(4)+'-'+kind;internal='5432' if kind=='pg' else '6379'
 run('docker','run','-d','--rm','--name',name,'-p','127.0.0.1::'+internal,*args,image);containers.append(name)
 return json.loads(run('docker','inspect',name))[0]['NetworkSettings']['Ports'][internal+'/tcp'][0]['HostPort']
try:
 with tempfile.TemporaryDirectory(prefix='activity-proof-') as td:
  tmp=Path(td);commit=run('git','-C',str(ROOT),'rev-parse','--short=8','HEAD')+'+activity'
  for binary,pkg in [('temenos','./cmd/server'),('keryx','./cmd/keryx')]:run('go','build','-ldflags','-X main.buildCommit='+commit,'-o',str(tmp/binary),pkg,cwd=ROOT/'server',env=clean)
  pg=launch('pg','postgres:16-alpine',['-e','POSTGRES_PASSWORD=proof','-e','POSTGRES_DB=proof']);redis=launch('redis','redis:7-alpine',[])
  for _ in range(100):
   if subprocess.run(['docker','exec',containers[0],'pg_isready','-U','postgres','-q'],capture_output=True).returncode==0:break
   time.sleep(.25)
  with socket.socket() as s:s.bind(('127.0.0.1',0));port=s.getsockname()[1]
  base=f'http://127.0.0.1:{port}'
  env={**clean,'DATABASE_URL':f'postgres://postgres:proof@localhost:{pg}/proof?sslmode=disable','REDIS_URL':'localhost:'+redis,'JWT_SECRET':secrets.token_urlsafe(40),'PORT':str(port),'TICK_SECONDS':'1800','MAP_WIDTH':'56','MAP_HEIGHT':'40','WORLD_NAME':'March selection proof','POLEIA_WORLD_START_WANAXES':'1','STATIC_DIR':str(ROOT/'web/static'),'TEMPLATE_DIR':str(ROOT/'web/templates'),'CHRONICLE_DIR':str(tmp/'chronicles'),'REPORTS_DIR':str(tmp/'reports')}
  assert not (ROOT/'server/.env').exists()
  proc=subprocess.Popen([str(tmp/'temenos')],cwd=ROOT/'server',env=env,stdout=(OUT/'server.log').open('w'),stderr=subprocess.STDOUT)
  def api(path,method='GET',data=None,token=None):
   req=urllib.request.Request(base+path,method=method,data=None if data is None else json.dumps(data).encode(),headers={'Content-Type':'application/json',**({'Authorization':'Bearer '+token} if token else {})})
   try:return json.load(urllib.request.urlopen(req,timeout=30))
   except urllib.error.HTTPError as e:raise RuntimeError(str(e)+' '+e.read().decode()) from e
  for _ in range(120):
   try:health=api('/healthz');break
   except Exception:
    if proc.poll() is not None:raise RuntimeError('server exited')
    time.sleep(.5)
  assert health['commit']==commit and health['migration']==216,health
  world=api('/api/v1/worlds')[0]['id'];wp='/api/v1/worlds/'+world
  def sql(q):return run('docker','exec',containers[0],'psql','-U','postgres','-d','proof','-At','-v','ON_ERROR_STOP=1','-c',q)
  observations=[]
  for mode in ['api','web','keryx']:
   username='Activity-'+mode+'-'+secrets.token_hex(4)
   token=api('/api/v1/auth/register','POST',{'username':username,'password':secrets.token_urlsafe(30)})['access_token']
   def stamp():return json.loads(sql(f"SELECT json_build_object('last_seen_at',last_seen_at,'row_version',xmin::text) FROM players WHERE username='{username}'"))
   before=stamp();assert before['last_seen_at'] is None,before
   if mode=='api':
    try:api('/api/v1/auth/me',token='invalid')
    except RuntimeError:pass
    assert stamp()==before,'invalid request recorded activity'
    response=api('/api/v1/auth/me',token=token);assert 'last_seen_at' not in response,'private activity leaked in auth view'
   elif mode=='web':
    req=urllib.request.Request(base+'/play',headers={'Cookie':'poleia_token='+token})
    with urllib.request.urlopen(req,timeout=30) as page:assert page.status==200
   else:
    cfg=tmp/'activity-config.json';cfg.write_text(json.dumps({'server':base,'token':token,'world_id':world}));cfg.chmod(0o600)
    result=subprocess.run([str(tmp/'keryx'),'unit','list'],env={**clean,'POLEIA_CONFIG':str(cfg)},text=True,capture_output=True)
    assert result.returncode==0,result.stderr
    (OUT/'keryx.json').write_text(json.dumps({'exit_code':result.returncode,'stdout':result.stdout,'stderr':result.stderr},indent=2))
   after=stamp();assert after['last_seen_at'] is not None,after
   api('/api/v1/auth/me',token=token);assert stamp()==after,'repeat wrote same player'
   observations.append({'mode':mode,'before':before,'after':after,'repeat_unchanged':True})
  proc.terminate();proc.wait(timeout=20);proc=None
  # All offline, with existing accounts preserved. Down drops observations by definition.
  count=sql('SELECT count(*) FROM players')
  dsn=env['DATABASE_URL']
  run('migrate','-path','db/migrations','-database',dsn,'down','1',cwd=ROOT/'server')
  assert sql("SELECT count(*) FROM information_schema.columns WHERE table_name='players' AND column_name='last_seen_at'")=='0'
  assert sql('SELECT count(*) FROM players')==count
  run('migrate','-path','db/migrations','-database',dsn,'up',cwd=ROOT/'server')
  assert sql("SELECT version::text||'|'||dirty::text FROM schema_migrations")=='216|false'
  assert sql('SELECT count(*) FROM players WHERE last_seen_at IS NOT NULL')=='0'
  assert sql('SELECT count(*) FROM players')==count
  assert source_before==hashes(),'production sources changed during acceptance'
  (OUT/'proof.json').write_text(json.dumps({'health':health,'observations':observations,'offline_schema_cycle':'216→215→216','source_hashes_unchanged':len(source_before),'accounts_preserved':int(count),'reintroduced_stamps_null':True},indent=2)+'\n')
  print('PASS real HTTP/Bearer, web cookie and Keryx; repeat row unchanged; offline216→215→216/accounts preserved/NULL stamps')
finally:
 if proc is not None:
  proc.terminate()
  try:proc.wait(timeout=20)
  except subprocess.TimeoutExpired:proc.kill();proc.wait()
 for name in reversed(containers):subprocess.run(['docker','rm','-f',name],capture_output=True)
