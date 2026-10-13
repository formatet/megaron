#!/usr/bin/env python3
"""Natural register/join/found and real worker SilverAuditV2, read-only inspection.
No SQL fixture/writes, request interception, supplied payloads or CSS overrides.
"""
import hashlib,json,os,pathlib,secrets,socket,subprocess,tempfile,time,urllib.request
HERE=pathlib.Path(__file__).resolve().parent;ROOT=HERE.parents[2]
OUT=HERE/os.environ.get('SILVER_AUDIT_NATIVE_OUT','native');OUT.mkdir(parents=True,exist_ok=True)
def run(*a,**kw):return subprocess.check_output(a,text=True,**kw).strip()
commit=run('git','-C',str(ROOT),'rev-parse','--short=8','HEAD')
diff=hashlib.sha256(subprocess.check_output(['git','-C',str(ROOT),'diff','HEAD','--','server','web'])).hexdigest()
assert run('git','-C',str(ROOT),'diff','HEAD','--','server','web')=='', 'commit production before native proof'
containers=[];proc=browser=pw=None
prefix='audit-native-'+secrets.token_hex(4)
def launch(service,image,args):
 name=prefix+'-'+service;port='5432' if service=='pg' else '6379'
 run('docker','run','-d','--rm','--name',name,'-p','127.0.0.1::'+port,*args,image);containers.append(name)
 return int(json.loads(run('docker','inspect',name))[0]['NetworkSettings']['Ports'][port+'/tcp'][0]['HostPort'])
try:
 with tempfile.TemporaryDirectory(prefix='audit-native-') as tmp:
  tmp=pathlib.Path(tmp);clean={k:os.environ[k] for k in ('HOME','PATH')};clean['CGO_ENABLED']='0'
  run('go','build','-ldflags','-X main.buildCommit='+commit,'-o',str(tmp/'temenos'),'./cmd/server',cwd=ROOT/'server',env=clean)
  run('go','build','-o',str(tmp/'keryx'),'./cmd/keryx',cwd=ROOT/'server',env=clean)
  pg=launch('pg','postgres:16-alpine',['-e','POSTGRES_PASSWORD=native-proof','-e','POSTGRES_DB=native_proof'])
  redis=launch('redis','redis:7-alpine',[])
  for _ in range(120):
   if subprocess.run(['docker','exec',containers[0],'pg_isready','-U','postgres','-q'],capture_output=True).returncode==0:break
   time.sleep(.25)
  time.sleep(1)
  with socket.socket() as sock:sock.bind(('127.0.0.1',0));port=sock.getsockname()[1]
  base=f'http://127.0.0.1:{port}'
  env={**clean,'DATABASE_URL':f'postgres://postgres:native-proof@localhost:{pg}/native_proof?sslmode=disable','REDIS_URL':f'localhost:{redis}',
   'JWT_SECRET':secrets.token_urlsafe(40),'PORT':str(port),'TICK_SECONDS':'2','MAP_WIDTH':'64','MAP_HEIGHT':'64',
   'WORLD_NAME':'Spear proof','POLEIA_WORLD_START_WANAXES':'1','STATIC_DIR':str(ROOT/'web/static'),'TEMPLATE_DIR':str(ROOT/'web/templates'),
   'CHRONICLE_DIR':str(tmp/'chronicles'),'REPORTS_DIR':str(tmp/'reports')}
  assert not (ROOT/'server/.env').exists()
  proc=subprocess.Popen([str(tmp/'temenos')],cwd=ROOT/'server',env=env,stdout=(OUT/'server.log').open('w'),stderr=subprocess.STDOUT)
  def api(path,method='GET',data=None,token=None):
   req=urllib.request.Request(base+path,method=method,data=None if data is None else json.dumps(data).encode(),headers={'Content-Type':'application/json',**({'Authorization':'Bearer '+token} if token else {})})
   with urllib.request.urlopen(req,timeout=30) as response:return json.load(response)
  for _ in range(120):
   try:health=api('/healthz');break
   except Exception:
    if proc.poll() is not None:raise RuntimeError('native server exited')
    time.sleep(.5)
  else:raise RuntimeError('native startup timeout')
  assert health['commit']==commit and health['migration']==179,health
  worlds=api('/api/v1/worlds');world=(worlds if isinstance(worlds,list) else worlds['worlds'])[0]['id'];wp='/api/v1/worlds/'+world
  token=api('/api/v1/auth/register','POST',{'username':'Spear-'+secrets.token_hex(4),'password':secrets.token_urlsafe(30)})['access_token']
  api(wp+'/join','POST',{},token)
  founder=api(wp+'/founding/status',token=token)
  assert founder['silver']['amount']==480 and founder['silver']['rate_per_tick']==0 and 'ticks_left' not in founder['silver'],founder
  before=api(wp+'/units',token=token)['units']
  assert all(u['upkeep_context']=='subsistence' and u['upkeep_silver_per_tick']==0 for u in before if u['type']=='spearman'),before
  cfg=tmp/'config.json';cfg.write_text(json.dumps({'server':base,'world_id':world,'token':token}));cfg.chmod(0o600)
  cli_env={**clean,'POLEIA_CONFIG':str(cfg)}
  before_cli=run(str(tmp/'keryx'),'unit','list',env=cli_env)
  assert 'Upkeep after settling: 1 grain, 0 silver per day' in before_cli,before_cli
  (OUT/'keryx-founder.txt').write_text(before_cli+'\n')
  def sql(query):
   data=subprocess.check_output(['docker','exec','-i',containers[0],'psql','-U','postgres','-d','native_proof','-qAt'],input='BEGIN READ ONLY;'+query+';COMMIT;',text=True)
   return json.loads(data.strip())
  def audit():
   return sql("SELECT json_build_object('id',id,'payload',payload) FROM events WHERE world_id='"+world+"' AND event_type='SilverAuditV2' ORDER BY id DESC LIMIT 1")
  for _ in range(30):
   try:opening=audit();break
   except json.JSONDecodeError:time.sleep(.5)
  else:raise AssertionError('worker emitted no opening V2')
  assert opening['payload']['founder_silver']==480 and opening['payload']['total_silver']==480,opening
  founded=api(wp+'/founding/settle','POST',{'name':'Phaistos'},token)
  for _ in range(30):
   after=audit()
   if after['id']>opening['id'] and after['payload']['founder_silver']==0:break
   time.sleep(.5)
  else:raise AssertionError('worker emitted no postfounding V2')
  destroyed=sql("SELECT COALESCE(sum((payload->>'silver_destroyed')::float),0) FROM events WHERE world_id='"+world+"' AND event_type='UpkeepSettled' AND id>"+str(opening['id'])+" AND id<"+str(after['id']))
  expected=founded['silver_total']-destroyed
  assert abs(after['payload']['total_silver']-expected)<1e-7,(after,expected)
  assert after['payload']['carried_silver']==0 and after['payload']['mined_since_last']==0,after
  assert after['payload']['wanax_distribution']['count']==1,after
  version=sql('SELECT json_build_object(\'version\',version,\'dirty\',dirty) FROM schema_migrations')
  assert version=={'version':179,'dirty':False},version
  (OUT/'verification.json').write_text(json.dumps({'health':health,'migration':version,'production_diff_sha256':diff,'natural_register_join_founding':True,'sql_fixtures_or_writes':False,'opening':opening,'first_founding':founded,'after':after,'upkeep_destroyed':destroyed,'expected_total':expected,'world_state':api(wp)['state']},indent=2)+'\n')
  print('PASS actual server '+str(health)+': natural founder480 → firstfound '+str(founded['silver_total'])+'; real tick worker V2 total='+str(expected)+' after salary '+str(destroyed)+'; SQL inspection READ ONLY')
finally:
 if proc:
  proc.terminate()
  try:proc.wait(timeout=10)
  except subprocess.TimeoutExpired:proc.kill();proc.wait()
 for name in containers:subprocess.run(['docker','rm','-fv',name],capture_output=True)
