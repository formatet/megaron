#!/usr/bin/env python3
"""Natural register/join; actual Host march preview API, CLI and Firefox.
No SQL fixture/writes, request interception, supplied payloads or CSS overrides.
"""
import hashlib,json,os,pathlib,secrets,socket,subprocess,tempfile,time,urllib.request
HERE=pathlib.Path(__file__).resolve().parent;ROOT=HERE.parents[2]
OUT=HERE/os.environ.get('STOCK_PROOF_OUT','native');OUT.mkdir(parents=True,exist_ok=True)
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
  (OUT/'cost-catalogues.json').write_text(json.dumps({'buildings':api('/api/v1/buildings'),'units':api('/api/v1/units')},indent=2)+'\n')
  choices={};trials=[]
  for i in range(30):
   fp=api(wp+'/founding/status',token=token);tiles=api(wp+'/map',token=token);tiles=tiles if isinstance(tiles,list) else tiles['tiles']
   tile=next(t for t in tiles if t['q']==fp['q'] and t['r']==fp['r']);kind='coast' if tile.get('coastal') else 'inland'
   trials.append({'kind':kind,'terrain':tile['terrain']});choices.setdefault(kind,(token,fp))
   if len(choices)==2:break
   token=api('/api/v1/auth/register','POST',{'username':'Stock-'+secrets.token_hex(4),'password':secrets.token_urlsafe(30)})['access_token'];api(wp+'/join','POST',{},token)
  assert len(choices)==2,trials
  from playwright.sync_api import sync_playwright
  pw=sync_playwright().start();browser=pw.firefox.launch();captures=[];founded=[]
  for kind,(token,fp) in choices.items():
   cfg=tmp/'config.json';cfg.write_text(json.dumps({'server':base,'world_id':world,'token':token}));cfg.chmod(0o600);cli_env={**clean,'POLEIA_CONFIG':str(cfg)}
   assert fp['grain']['amount']==1200 and fp['silver']['amount']==480,fp
   preview=api(wp+f"/colonize-preview?q={fp['q']}&r={fp['r']}&pop={fp['population']}&seed={fp['grain']['amount']}&starter_farm=1",token=token)
   assert preview['grain']['seed']==1200,preview
   default=api(wp+f"/colonize-preview?q={fp['q']}&r={fp['r']}",token=token)
   assert default['grain']['seed']==0,default
   for mode,width,height in [('desktop',1280,900),('mobile',390,844)]:
    ctx=browser.new_context(viewport={'width':width,'height':height});ctx.add_cookies([{'name':'poleia_token','value':token,'url':base}]);ctx.add_init_script('localStorage.setItem("poleia_token",'+json.dumps(token)+')')
    page=ctx.new_page();errors=[];responses=[];page.on('pageerror',lambda e:errors.append(str(e)))
    def record(response):
     if '/colonize-preview?' in response.url and response.status==200:responses.append({'url':response.url,'body':response.json()})
    page.on('response',record)
    page.goto(base+'/world/'+world+'/map',wait_until='networkidle')
    if page.locator('#dc-dismiss-all').is_visible():page.locator('#dc-dismiss-all').click()
    for brief in page.locator('.lb-dismiss').all():
     if brief.is_visible():brief.click()
    page.wait_for_function('document.getElementById("gt-wanax")?.textContent.includes("Wanax")')
    # Read the renderer camera/hex geometry; click the real Host on the canvas.
    point=page.evaluate('''async h=>{const m=await import('/static/js/megaron/render/map.js');const {State}=await import('/static/js/megaron/state.js');const p=m.hexPx(h.q,h.r);const b=m.canvas.getBoundingClientRect();return {x:b.x+p.x*m.SCALE*State.camera.zoom+State.camera.x,y:b.y+p.y*m.SCALE*State.camera.zoom+State.camera.y}}''',{'q':fp['q'],'r':fp['r']})
    page.mouse.click(point['x'],point['y']);page.locator('#ip-settle-btn').wait_for()
    page.wait_for_function('document.getElementById("ip-found-summary")?.textContent && !document.getElementById("ip-found-summary").textContent.includes("Fetching")')
    page.locator('#ip-host-details summary').click();page.locator('#ip-found-preview').scroll_into_view_if_needed()
    assert responses and responses[-1]['body']['grain']['seed']==1200,responses
    page.screenshot(path=str(OUT/(kind+'-'+mode+'-before.png')))
    captures.append({'kind':kind,'mode':mode,'phase':'before','preview_response':responses[-1],'host_text':page.locator('#ip-body-extra').inner_text(),'errors':errors})
    assert not errors,errors;ctx.close()
   cli=run(str(tmp/'keryx'),'founding','settle','--name','Stock-'+kind,'--yes',env=cli_env)
   assert '1200 grain, 480 silver' in cli and 'founding adds no extra grain or silver' in cli,cli
   (OUT/(kind+'-keryx-founding.txt')).write_text(cli+'\n')
   city=next(c for c in api(wp+'/provinces',token=token) if c.get('own'));pp=wp+'/provinces/'+city['id']
   goods=api(pp+'/goods',token=token);stock={g['key']:g for g in goods}
   assert stock['grain']['amount']==1200 and stock['silver']['amount']==480 and stock['silver']['cap']==960,stock
   assert stock['timber']['amount']==0 and stock['stone']['amount']==0,stock
   result={'kind':kind,'founder_before':fp,'forecast':preview,'default_colony_forecast':default,'goods':goods}
   for mode,width,height in [('desktop',1280,900),('mobile',390,844)]:
    ctx=browser.new_context(viewport={'width':width,'height':height});ctx.add_cookies([{'name':'poleia_token','value':token,'url':base}]);ctx.add_init_script('localStorage.setItem("poleia_token",'+json.dumps(token)+')')
    page=ctx.new_page();errors=[];page.on('pageerror',lambda e:errors.append(str(e)));page.goto(base+'/world/'+world+'/map',wait_until='networkidle')
    if page.locator('#dc-dismiss-all').is_visible():page.locator('#dc-dismiss-all').click()
    for brief in page.locator('.lb-dismiss').all():
     if brief.is_visible():brief.click()
    page.locator('#trig-economy').click();page.locator('#drawer-economy .dtab[data-tab="goods"]').click()
    page.wait_for_timeout(500)
    for brief in page.locator('#drawer-economy .lb-dismiss').all():
     if brief.is_visible():brief.click()
    page.locator('#drawer-economy tr').filter(has=page.locator('td').filter(has_text='Grain')).first.scroll_into_view_if_needed()
    text=page.locator('#drawer-economy').inner_text();assert '480' in text and ('1,200' in text or '1200' in text),text
    page.screenshot(path=str(OUT/(kind+'-'+mode+'-after.png')));captures.append({'kind':kind,'mode':mode,'phase':'after','text':text,'errors':errors});assert not errors,errors;ctx.close()
   founded.append(result)
  (OUT/'verification.json').write_text(json.dumps({'health':health,'production_diff_sha256':diff,'natural_register_join_cli_founding':True,'trials':trials,'foundings':founded,'captures':captures,'sql_fixtures_or_writes':False,'request_overrides':False},indent=2)+'\n')
  print('PASS natural inland+coast: forecast1200/colony0 -> CLI founding ->1200grain/480silver/cap960/0timber/0stone ->Firefox1280/390')

finally:
 if browser:browser.close()
 if pw:pw.stop()
 if proc:
  proc.terminate()
  try:proc.wait(timeout=10)
  except subprocess.TimeoutExpired:proc.kill();proc.wait()
 for name in containers:subprocess.run(['docker','rm','-fv',name],capture_output=True)
