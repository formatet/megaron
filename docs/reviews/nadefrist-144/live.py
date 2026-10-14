#!/usr/bin/env python3
"""Natural register/join; actual Host march preview API, CLI and Firefox.
No SQL fixture/writes, request interception, supplied payloads or CSS overrides.
"""
import hashlib,json,os,pathlib,secrets,socket,subprocess,tempfile,time,urllib.request
HERE=pathlib.Path(__file__).resolve().parent;ROOT=HERE.parents[2]
OUT=HERE/os.environ.get('GRACE_PROOF_OUT','native');OUT.mkdir(parents=True,exist_ok=True)
def run(*a,**kw):return subprocess.check_output(a,text=True,**kw).strip()
commit=run('git','-C',str(ROOT),'rev-parse','--short=8','HEAD')
diff=hashlib.sha256(subprocess.check_output(['git','-C',str(ROOT),'diff','HEAD','--','server','web'])).hexdigest()
assert run('git','-C',str(ROOT),'diff','HEAD','--','server','web')=='', 'commit production before native proof'
containers=[];proc=browser=pw=None
prefix='march-live-'+secrets.token_hex(4)
def launch(service,image,args):
 name=prefix+'-'+service;port='5432' if service=='pg' else '6379'
 run('docker','run','-d','--rm','--name',name,'-p','127.0.0.1::'+port,*args,image);containers.append(name)
 return int(json.loads(run('docker','inspect',name))[0]['NetworkSettings']['Ports'][port+'/tcp'][0]['HostPort'])
try:
 with tempfile.TemporaryDirectory(prefix='march-native-') as tmp:
  tmp=pathlib.Path(tmp);clean={k:os.environ[k] for k in ('HOME','PATH')};clean['CGO_ENABLED']='0'
  run('go','build','-ldflags','-X main.buildCommit='+commit,'-o',str(tmp/'temenos'),'./cmd/server',cwd=ROOT/'server',env=clean)
  run('go','build','-o',str(tmp/'keryx'),'./cmd/keryx',cwd=ROOT/'server',env=clean)
  pg=launch('pg','postgres:16-alpine',['-e','POSTGRES_PASSWORD=native-proof','-e','POSTGRES_DB=native_proof'])
  redis=launch('redis','redis:7-alpine',[])
  for _ in range(120):
   if subprocess.run(['docker','exec',containers[0],'pg_isready','-h','127.0.0.1','-U','postgres','-q'],capture_output=True).returncode==0:break
   time.sleep(.25)
  time.sleep(1)
  with socket.socket() as sock:sock.bind(('127.0.0.1',0));port=sock.getsockname()[1]
  base=f'http://127.0.0.1:{port}'
  env={**clean,'DATABASE_URL':f'postgres://postgres:native-proof@localhost:{pg}/native_proof?sslmode=disable','REDIS_URL':f'localhost:{redis}',
   'JWT_SECRET':secrets.token_urlsafe(40),'PORT':str(port),'TICK_SECONDS':'3600','MAP_WIDTH':'64','MAP_HEIGHT':'64',
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
  notes=api(wp+'/notifications?kind=OpeningDispatch',token=token)['notifications'];assert len(notes)==1,notes
  note=notes[0];paragraphs=note['body']['paragraphs'];needle='for 144 days after founding'
  assert any(needle in p for p in paragraphs),paragraphs
  cfg=tmp/'config.json';cfg.write_text(json.dumps({'server':base,'world_id':world,'token':token}));cfg.chmod(0o600)
  cli=run(str(tmp/'keryx'),'notifications',env={**clean,'POLEIA_CONFIG':str(cfg)});assert needle in cli,cli
  (OUT/'keryx-notifications.txt').write_text(cli+'\n')
  from playwright.sync_api import sync_playwright
  pw=sync_playwright().start();browser=pw.firefox.launch();captures=[]
  for mode,width,height in [('desktop',1280,900),('mobile',390,844)]:
   ctx=browser.new_context(viewport={'width':width,'height':height});ctx.add_cookies([{'name':'poleia_token','value':token,'url':base}]);ctx.add_init_script('localStorage.setItem("poleia_token",'+json.dumps(token)+')')
   page=ctx.new_page();errors=[];page.on('pageerror',lambda e:errors.append(str(e)))
   page.goto(base+'/world/'+world+'/map',wait_until='networkidle')
   if mode=='desktop':page.locator('.dispatch-chip').filter(has_text=note['body']['title']).click()
   else:
    page.evaluate("openDrawer('notif')");page.locator('#notif-body .notif-list-item').filter(has_text=note['body']['title']).click()
   page.wait_for_function("document.getElementById('dispatch-window-overlay')?.classList.contains('open')")
   text=page.locator('#dw-body').inner_text();assert needle in text and all(p in text for p in paragraphs),text
   page.screenshot(path=str(OUT/(mode+'.png')))
   captures.append({'mode':mode,'text':text,'errors':errors});assert not errors,errors;ctx.close()
  (OUT/'verification.json').write_text(json.dumps({'health':health,'natural_register_join':True,'notification':note,'captures':captures,'sql_fixtures_or_writes':False,'request_overrides':False},indent=2)+'\n')
  print('PASS144days: natural register/join -> actual notifications API + CLI + Firefox1280/390')

finally:
 if browser:browser.close()
 if pw:pw.stop()
 if proc:
  proc.terminate()
  try:proc.wait(timeout=10)
  except subprocess.TimeoutExpired:proc.kill();proc.wait()
 for name in containers:subprocess.run(['docker','rm','-fv',name],capture_output=True)
