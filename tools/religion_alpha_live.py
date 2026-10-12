#!/usr/bin/env python3
"""Fresh isolated runtime: natural join, Host forecast in API/Keryx/web, real march.
Normal registration/join/founding; explicit own-DB temple/stock fixture before action.
No SQL writes during actions, HTTP overrides or shared acceptance runtime.
"""
import json, os, pathlib, secrets, socket, subprocess, tempfile, time, urllib.request
ROOT=pathlib.Path(__file__).resolve().parents[1]
OUT=ROOT/'docs/reviews/religion-alfa/live-a'
OUT.mkdir(parents=True,exist_ok=True)
commit=subprocess.check_output(['git','-C',str(ROOT),'rev-parse','--short=7','HEAD'],text=True).strip()
containers=[];proc=None;browser=None;playwright=None
prefix='megaron-religion-a-'+secrets.token_hex(4)
def run(*args,**kw):return subprocess.check_output(args,text=True,**kw).strip()
def launch(service,image,args):
 name=prefix+'-'+service;port='5432' if service=='pg' else '6379'
 run('docker','run','-d','--rm','--name',name,'-p','127.0.0.1::'+port,*args,image);containers.append(name)
 bindings=json.loads(run('docker','inspect',name))[0]['NetworkSettings']['Ports']
 return int(bindings[port+'/tcp'][0]['HostPort'])
try:
 with tempfile.TemporaryDirectory(prefix='megaron-ro1-live-') as tmp:
  tmp=pathlib.Path(tmp)
  clean={k:os.environ[k] for k in ('HOME','PATH')};clean['CGO_ENABLED']='0'
  run('go','build','-ldflags','-X main.buildCommit='+commit,'-o',str(tmp/'temenos'),'./cmd/server',cwd=ROOT/'server',env=clean)
  run('go','build','-o',str(tmp/'keryx'),'./cmd/keryx',cwd=ROOT/'server',env=clean)
  pg=launch('pg','postgres:16-alpine',['-e','POSTGRES_PASSWORD=ro1-proof-pw','-e','POSTGRES_DB=ro1_proof'])
  redis=launch('redis','redis:7-alpine',[])
  for _ in range(80):
   if subprocess.run(['docker','exec',containers[0],'pg_isready','-U','postgres','-q'],capture_output=True).returncode==0:break
   time.sleep(.25)
  with socket.socket() as sock:sock.bind(('127.0.0.1',0));port=sock.getsockname()[1]
  base=f'http://127.0.0.1:{port}'
  env={**clean,'DATABASE_URL':f'postgres://postgres:ro1-proof-pw@localhost:{pg}/ro1_proof?sslmode=disable','REDIS_URL':f'localhost:{redis}',
   'JWT_SECRET':secrets.token_urlsafe(40),'PORT':str(port),'TICK_SECONDS':'60','MAP_WIDTH':'64','MAP_HEIGHT':'48',
   'WORLD_NAME':'Religion alpha A proof','POLEIA_WORLD_START_WANAXES':'1','STATIC_DIR':str(ROOT/'web/static'),'TEMPLATE_DIR':str(ROOT/'web/templates'),
   'CHRONICLE_DIR':str(tmp/'chronicles'),'REPORTS_DIR':str(tmp/'reports')}
  assert not (ROOT/'server/.env').exists()
  proc=subprocess.Popen([str(tmp/'temenos')],cwd=ROOT/'server',env=env,stdout=(OUT/'server.log').open('w'),stderr=subprocess.STDOUT)
  def api(path,method='GET',data=None,token=None):
   req=urllib.request.Request(base+path,method=method,data=None if data is None else json.dumps(data).encode(),headers={'Content-Type':'application/json',**({'Authorization':'Bearer '+token} if token else {})})
   with urllib.request.urlopen(req,timeout=30) as response:return json.load(response)
  for _ in range(120):
   try:health=api('/healthz');break
   except Exception:
    if proc.poll() is not None:raise RuntimeError('server exited')
    time.sleep(.5)
  else:raise RuntimeError('server timeout')
  assert health['commit']==commit and health['migration']==175,health
  ws=api('/api/v1/worlds');world=(ws if isinstance(ws,list) else ws['worlds'])[0]['id'];wp='/api/v1/worlds/'+world
  username='Alpha-A-'+secrets.token_hex(4)
  token=api('/api/v1/auth/register','POST',{'username':username,'password':secrets.token_urlsafe(30)})['access_token']
  api(wp+'/join','POST',{},token)
  founded=api(wp+'/founding/settle','POST',{'name':'Phaistos'},token)
  markers=api(wp+'/provinces',token=token)
  city=next(m for m in markers if m.get('own'))
  settlement=city['settlement_id'];province=city['id']
  # Fixture is explicit, scoped to this newly registered city/realm, before
  # user action; this proves temple interaction, not natural temple construction.
  sql=f"""INSERT INTO buildings(settlement_id,building_type,level) VALUES('{settlement}','temple',3);
  UPDATE settlement_goods SET amount=100,rate=0,calc_tick=current_world_tick() WHERE settlement_id='{settlement}' AND good_key IN('grain','timber','fish','silver');
  UPDATE player_world_records SET kharis_amount=90,kharis_rate=0,kharis_calc_tick=current_world_tick() WHERE world_id='{world}' AND settlement_id='{settlement}';"""
  run('docker','exec',containers[0],'psql','-U','postgres','-d','ro1_proof','-v','ON_ERROR_STOP=1','-c',sql)
  cfg=tmp/'config.json';cfg.write_text(json.dumps({'server':base,'world_id':world,'province_id':province,'token':token}));cfg.chmod(0o600)
  clienv={**clean,'POLEIA_CONFIG':str(cfg)}
  cli=run(str(tmp/'keryx'),'rite','--list',env=clienv)
  assert all(name in cli for name in ["Potnia's Vision","Mistress of the Walls","Green Shoots"]),cli
  assert 'Britomartis' not in cli and 'Poseidon' not in cli,cli
  (OUT/'keryx-before.txt').write_text(cli+'\n')
  from playwright.sync_api import sync_playwright
  playwright=sync_playwright().start();browser=playwright.firefox.launch()
  captures=[];action=None
  for label,width,height in [('desktop',1280,900),('mobile',390,844)]:
   context=browser.new_context(viewport={'width':width,'height':height})
   context.add_cookies([{'name':'poleia_token','value':token,'url':base}]);context.add_init_script('localStorage.setItem("poleia_token",'+json.dumps(token)+')')
   page=context.new_page();errors=[];page.on('pageerror',lambda e:errors.append(str(e)))
   page.goto(base+'/world/'+world+'/map',wait_until='networkidle')
   if page.locator('#dc-dismiss-all').is_visible():page.locator('#dc-dismiss-all').click()
   for brief in page.locator('.lb-dismiss').all():
    if brief.is_visible():brief.click()
   page.evaluate("openDrawer('kult')")
   page.wait_for_function("document.getElementById('kult-body')?.textContent.includes('Mistress of the Walls')")
   for brief in page.locator('.lb-dismiss').all():
    if brief.is_visible():brief.click()
   page.evaluate("async()=>await Promise.all([...document.querySelectorAll('.drawer.open')].flatMap(d=>d.getAnimations()).map(a=>a.finished.catch(()=>{})))")
   text=page.locator('#kult-body').inner_text()
   assert all(name in text for name in ["Potnia's Vision","Mistress of the Walls","Green Shoots"]),text
   assert 'quarter stronger' in text and 'unknown ground' in text,text
   page.screenshot(path=str(OUT/(label+'.png')))
   if label=='mobile':
    page.locator('#kult-body').evaluate('(el)=>{el.scrollTop=el.scrollHeight;}')
    page.evaluate('async()=>await new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)))')
    page.screenshot(path=str(OUT/'mobile-prayers.png'))
   if label=='desktop':
    attempts=[]
    for _ in range(25):
     button=page.locator('button[onclick="okRite(\'minoan_potnia_walls\')"]')
     with page.expect_response(lambda r:r.request.method=='POST' and r.url.endswith('/rite')) as reply:button.click()
     response=reply.value;payload=response.json();assert response.status==200,payload;attempts.append(payload)
     page.locator('#kult-body [data-inline-result]').wait_for()
     if payload['success']:break
    else:raise AssertionError('no success observed in 25 real attempts')
    page.wait_for_function("document.getElementById('kult-body')?.textContent.includes('on cooldown')")
    page.screenshot(path=str(OUT/'desktop-active.png'))
    current=api(wp+'/provinces/'+province,token=token)['settlement']['available_prayers']
    by_id={p['id']:p for p in current}
    assert by_id['minoan_potnia_walls']['active_remaining_game_days']==6,by_id
    assert by_id['minoan_potnia_vision_v2']['cooldown_remaining_game_days']==24,by_id
    assert not by_id['minoan_potnia_vision_v2']['affordable'],by_id
    action={'attempts':attempts,'after':current,'inline':page.locator('#kult-body [data-inline-result]').inner_text(),'sql_writes_during_action':False}
   captures.append({'mode':label,'errors':list(errors),'text':page.locator('#kult-body').inner_text()})
   context.close()
  after=run(str(tmp/'keryx'),'rite','--list',env=clienv);(OUT/'keryx-after.txt').write_text(after+'\n')
  assert '24 days' in after and '6 days' in after,after
  assert all(not c['errors'] for c in captures),captures
  (OUT/'verification.json').write_text(json.dumps({'health':health,'world':world,'username':username,'normal_register_join_founding':True,'fixture':'Own city temple level3, grain/timber/fish/silver100, realm kharis90/rate0 before actions; not natural temple build','sql_writes_during_actions':False,'payload_overrides':False,'captures':captures,'action':action},indent=2)+'\n')
  print('Firefox1280/390 temple + real Offer200/active6/shared24 + Keryx list; explicit pre-action fixture')
finally:
 if browser:browser.close()
 if playwright:playwright.stop()
 if proc:
  proc.terminate()
  try:proc.wait(timeout=10)
  except subprocess.TimeoutExpired:proc.kill();proc.wait()
 for name in containers:subprocess.run(['docker','rm','-fv',name],capture_output=True)
