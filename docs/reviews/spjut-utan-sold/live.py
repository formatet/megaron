#!/usr/bin/env python3
"""Natural register/join/found; actual unit API, Keryx and Firefox1280/390.
No SQL fixture/writes, request interception, supplied payloads or CSS overrides.
"""
import hashlib,json,os,pathlib,secrets,socket,subprocess,tempfile,time,urllib.request
HERE=pathlib.Path(__file__).resolve().parent;ROOT=HERE.parents[2]
OUT=HERE/os.environ.get('B_PROOF_OUT','live');OUT.mkdir(parents=True,exist_ok=True)
def run(*a,**kw):return subprocess.check_output(a,text=True,**kw).strip()
commit=run('git','-C',str(ROOT),'rev-parse','--short=8','HEAD')
diff=hashlib.sha256(subprocess.check_output(['git','-C',str(ROOT),'diff','HEAD','--','server','web'])).hexdigest()
assert run('git','-C',str(ROOT),'diff','HEAD','--','server','web')=='', 'commit production before native proof'
containers=[];proc=browser=pw=None
prefix='spjut-live-'+secrets.token_hex(4)
def launch(service,image,args):
 name=prefix+'-'+service;port='5432' if service=='pg' else '6379'
 run('docker','run','-d','--rm','--name',name,'-p','127.0.0.1::'+port,*args,image);containers.append(name)
 return int(json.loads(run('docker','inspect',name))[0]['NetworkSettings']['Ports'][port+'/tcp'][0]['HostPort'])
try:
 with tempfile.TemporaryDirectory(prefix='spjut-native-') as tmp:
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
  assert health['commit']==commit and health['migration']==178,health
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
  founded=api(wp+'/founding/settle','POST',{'name':'Phaistos'},token)
  markers=api(wp+'/provinces',token=token);city=next(m for m in markers if m.get('own'));province=city['id']
  cfg.write_text(json.dumps({'server':base,'world_id':world,'province_id':province,'token':token}));cfg.chmod(0o600)
  goods=api(wp+'/provinces/'+province+'/goods',token=token);silver=next(g for g in goods if g['key']=='silver')
  assert silver['amount']==(1278 if founded.get('coastal') else 730),silver
  units=api(wp+'/units',token=token)['units'];spears=[u for u in units if u['type']=='spearman']
  assert len(spears)==2 and all(u['upkeep_grain_per_tick']==.5 and u['upkeep_silver_per_tick']==0 and u['upkeep_context']=='active' for u in spears),spears
  cli=run(str(tmp/'keryx'),'unit','list',env=cli_env);assert cli.count('Upkeep: 0.5 grain, 0 silver per day')==2,cli
  (OUT/'keryx-units.txt').write_text(cli+'\n')
  from playwright.sync_api import sync_playwright
  pw=sync_playwright().start();browser=pw.firefox.launch();captures=[]
  for mode,width,height in [('desktop',1280,900),('mobile',390,844)]:
   ctx=browser.new_context(viewport={'width':width,'height':height})
   ctx.add_cookies([{'name':'poleia_token','value':token,'url':base}]);ctx.add_init_script('localStorage.setItem("poleia_token",'+json.dumps(token)+')')
   page=ctx.new_page();errors=[];page.on('pageerror',lambda e:errors.append(str(e)))
   page.goto(base+'/world/'+world+'/map',wait_until='networkidle')
   if page.locator('#dc-dismiss-all').is_visible():page.locator('#dc-dismiss-all').click()
   for brief in page.locator('.drawer.open .lb-dismiss').all():
    if brief.is_visible():brief.click()
   page.evaluate("openDrawer('war')")
   for u in spears:page.locator('#ucard-'+u['id']+' .unit-upkeep').wait_for()
   for brief in page.locator('.drawer.open .lb-dismiss').all():
    if brief.is_visible():brief.click()
   page.evaluate("async()=>await Promise.all([...document.querySelectorAll('.drawer.open')].flatMap(d=>d.getAnimations()).map(a=>a.finished.catch(()=>{})))")
   texts=[];rects=[]
   for u in spears:
    row=page.locator('#ucard-'+u['id']+' .unit-upkeep');row.scroll_into_view_if_needed();text=row.inner_text()
    assert text=='Upkeep: 0.5 grain, 0 silver per day',text
    rect=row.bounding_box();assert rect and rect['x']>=0 and rect['x']+rect['width']<=width+1,rect
    texts.append(text);rects.append(rect)
   page.screenshot(path=str(OUT/(mode+'.png')))
   (OUT/(mode+'-units.txt')).write_text(page.locator('#war-body').inner_text()+'\n')
   assert not errors,errors
   captures.append({'mode':mode,'viewport':[width,height],'texts':texts,'rects':rects,'page_errors':errors});ctx.close()
  (OUT/'verification.json').write_text(json.dumps({'health':health,'production_diff_sha256':diff,'natural_register_join_founding':True,'sql_fixtures_or_writes':False,'request_overrides':False,'founder':founder,'founder_units':before,'first_founding':founded,'first_silver':silver,'units':units,'captures':captures},indent=2)+'\n')
  print('PASS natural founder480/rate0 → firstfound730/1278; actual API/Keryx/Firefox1280/390 rations.5/salary0; noSQL/overrides')
finally:
 if browser:browser.close()
 if pw:pw.stop()
 if proc:
  proc.terminate()
  try:proc.wait(timeout=10)
  except subprocess.TimeoutExpired:proc.kill();proc.wait()
 for name in containers:subprocess.run(['docker','rm','-fv',name],capture_output=True)
