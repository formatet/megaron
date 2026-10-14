#!/usr/bin/env python3
"""Current code against a private read-only snapshot of the existing acceptance world.
The source containers are never changed. Login and real drawers; no payload overrides.
"""
import hashlib,json,os,pathlib,secrets,socket,subprocess,tempfile,time,urllib.request
from playwright.sync_api import sync_playwright
HERE=pathlib.Path(__file__).resolve().parent;ROOT=HERE.parents[2]
MODE=os.environ.get('ALPHA_PROOF_MODE','after');assert MODE in ('before','sources','after')
OUT=HERE/os.environ.get('ALPHA_PROOF_OUT',MODE);OUT.mkdir(parents=True,exist_ok=True)
def run(*a,**kw):return subprocess.check_output(a,text=True,**kw).strip()
commit=run('git','-C',str(ROOT),'rev-parse','--short=8','HEAD')
clean={k:os.environ[k] for k in ('HOME','PATH')};clean['CGO_ENABLED']='0'
containers=[];proc=None;prefix='alpha-proof-'+secrets.token_hex(4)
source_health=json.load(urllib.request.urlopen('http://localhost:8097/healthz'))
def launch(kind,image,args):
 name=prefix+'-'+kind;internal='5432' if kind=='pg' else '6379'
 run('docker','run','-d','--rm','--name',name,'-p','127.0.0.1::'+internal,*args,image);containers.append(name)
 return int(json.loads(run('docker','inspect',name))[0]['NetworkSettings']['Ports'][internal+'/tcp'][0]['HostPort'])
try:
 with tempfile.TemporaryDirectory(prefix='alpha-proof-') as scratch:
  scratch=pathlib.Path(scratch);dump=scratch/'acceptance.dump'
  with dump.open('wb') as f:subprocess.run(['docker','exec','megaron-acc-postgres-1','pg_dump','-Fc','--no-owner','--no-acl','-U','poleia','-d','poleia'],stdout=f,check=True)
  dump_hash=hashlib.sha256(dump.read_bytes()).hexdigest()
  pg=launch('pg','postgres:16-alpine',['-e','POSTGRES_USER=poleia','-e','POSTGRES_PASSWORD=private-proof','-e','POSTGRES_DB=poleia'])
  redis=launch('redis','redis:7-alpine',[])
  for _ in range(120):
   if subprocess.run(['docker','exec',containers[0],'pg_isready','-h','127.0.0.1','-U','poleia','-q'],capture_output=True).returncode==0:break
   time.sleep(.25)
  with dump.open('rb') as f:subprocess.run(['docker','exec','-i',containers[0],'pg_restore','-U','poleia','-d','poleia','--no-owner','--no-privileges'],stdin=f,check=True)
  binary=scratch/'temenos'
  run('go','build','-ldflags','-X main.buildCommit='+commit,'-o',str(binary),'./cmd/server',cwd=ROOT/'server',env=clean)
  with socket.socket() as sock:sock.bind(('127.0.0.1',0));port=sock.getsockname()[1]
  base=f'http://127.0.0.1:{port}'
  env={**clean,'DATABASE_URL':f'postgres://poleia:private-proof@localhost:{pg}/poleia?sslmode=disable','REDIS_URL':f'localhost:{redis}',
   'JWT_SECRET':secrets.token_urlsafe(40),'PORT':str(port),'TICK_SECONDS':'86400','STATIC_DIR':str(ROOT/'web/static'),'TEMPLATE_DIR':str(ROOT/'web/templates'),
   'CHRONICLE_DIR':str(scratch/'chronicles'),'REPORTS_DIR':str(scratch/'reports')}
  assert not (ROOT/'server/.env').exists()
  proc=subprocess.Popen([str(binary)],cwd=ROOT/'server',env=env,stdout=(OUT/'server.log').open('w'),stderr=subprocess.STDOUT)
  def api(path,data=None,token=None):
   req=urllib.request.Request(base+path,data=None if data is None else json.dumps(data).encode(),headers={'Content-Type':'application/json',**({'Authorization':'Bearer '+token} if token else {})})
   with urllib.request.urlopen(req,timeout=30) as response:return json.load(response)
  for _ in range(120):
   try:health=api('/healthz');break
   except Exception:
    if proc.poll() is not None:raise RuntimeError('private server exited')
    time.sleep(.5)
  else:raise RuntimeError('private startup timeout')
  assert health['commit']==commit and health['migration']==179,health
  if MODE=='after':assert not run('git','-C',str(ROOT),'diff','HEAD','--','server','web'),'commit changes before final proof'
  captures=[];errors=[]
  with sync_playwright() as pw:
   browser=pw.firefox.launch()
   for mode,width,height in [('desktop',1280,900),('mobile',390,844)]:
    context=browser.new_context(viewport={'width':width,'height':height});page=context.new_page();page.on('pageerror',lambda e:errors.append(str(e)))
    response=page.goto(base+'/alfatestinfo',wait_until='networkidle');assert response.status==200
    for img in page.locator('img').all():
     img.scroll_into_view_if_needed();page.wait_for_function('(el)=>el.complete && el.naturalWidth>0',arg=img.element_handle());img.evaluate('(el)=>el.decode()')
    images=page.locator('img').evaluate_all('(els)=>els.map(el=>({src:el.getAttribute("src"),width:el.naturalWidth,height:el.naturalHeight}))')
    assert len(images)==4 and all(i['width']>0 for i in images),images
    if MODE=='after':assert all(i['width']==1440 and i['height']==1000 for i in images),images
    metrics=page.evaluate('({viewport:innerWidth,width:document.documentElement.scrollWidth})');assert metrics['width']<=width+1,metrics
    text=page.locator('main').inner_text()
    if MODE=='after':assert page.locator('html').get_attribute('lang')=='sv' and 'keryx report' in text and 'Report' in text and 'ticks' not in text.lower(),text
    page.evaluate('scrollTo(0,0)');page.screenshot(path=str(OUT/(mode+'.png')),full_page=True);page.screenshot(path=str(OUT/(mode+'-top.png')))
    captures.append({'mode':mode,'status':response.status,'metrics':metrics,'text':text,'words':len(text.split()),'images':images})
    context.close()
   if MODE in ('sources','after'):
    token=api('/api/v1/auth/login',{'username_or_email':'Eirene','password':'acceptance-pw-123'})['access_token']
    worlds=api('/api/v1/worlds');worlds=worlds if isinstance(worlds,list) else worlds['worlds']
    choices=[]
    for candidate in worlds:
     wp='/api/v1/worlds/'+candidate['id'];provinces=api(wp+'/provinces',token=token);cities=[p for p in provinces if p.get('own')]
     if not cities:continue
     inbox=api(wp+'/messengers/inbox',token=token)
     sent=sum(len(api(wp+'/settlements/'+c['settlement_id']+'/messengers',token=token)) for c in cities)
     choices.append((len(inbox)+sent,len(cities),len(provinces),candidate['id'],cities))
    assert choices,'acceptance Eirene has no city'
    mail_count,_,_,world,own=max(choices,key=lambda c:c[:3])
    assert mail_count>0, 'acceptance has no real correspondence for the requested image'
    context=browser.new_context(viewport={'width':1440,'height':1000});context.add_cookies([{'name':'poleia_token','value':token,'url':base}]);context.add_init_script('localStorage.setItem("poleia_token",'+json.dumps(token)+')')
    page=context.new_page();page.on('pageerror',lambda e:errors.append(str(e)));page.goto(base+'/world/'+world+'/map',wait_until='networkidle')
    page.wait_for_function('document.getElementById("gt-wanax")?.textContent.includes("Wanax")');page.wait_for_timeout(3000)
    dismiss=page.locator('#dc-dismiss-all')
    if dismiss.is_visible():dismiss.click()
    for button in page.locator('.lb-dismiss').all():
     if button.is_visible():button.click()
    page.screenshot(path=str(OUT/'karta.png'))
    page.locator('#trig-city').click();page.locator('#drawer-city .dtab[data-tab="byggnader"]').click();page.wait_for_timeout(1200)
    for button in page.locator('.lb-dismiss').all():
     if button.is_visible():button.click()
    page.screenshot(path=str(OUT/'stad.png'))
    buildings=page.locator('#drawer-city').inner_text()
    page.locator('#drawer-city .dtab[data-tab="produktion"]').click();page.wait_for_timeout(1200);page.screenshot(path=str(OUT/'produktion.png'))
    production=page.locator('#drawer-city').inner_text()
    page.locator('#trig-diplomacy').click();page.wait_for_timeout(1200)
    for button in page.locator('.lb-dismiss').all():
     if button.is_visible():button.click()
    page.screenshot(path=str(OUT/'handel.png'))
    diplomacy=page.locator('#drawer-diplomacy').inner_text()
    assert 'buildings' in buildings.lower() and 'production' in production.lower(), (buildings,production)
    assert 'correspondence' in diplomacy.lower() and 'no correspondence yet' not in diplomacy.lower(),diplomacy
    (OUT/'game-views.json').write_text(json.dumps({'world':world,'correspondence_count':mail_count,'selection':[{ 'world':c[3],'correspondence':c[0],'own_cities':c[1]} for c in choices],'own_cities':own,'buildings':buildings,'production':production,'diplomacy':diplomacy},indent=2)+'\n')
    context.close()
   browser.close()
  assert not errors,errors
  (OUT/'verification.json').write_text(json.dumps({'health':health,'source_health':source_health,'snapshot_sha256':dump_hash,'source_read_only':True,'http_payload_overrides':False,'captures':captures,'errors':errors},indent=2)+'\n')
  print('PASS private acceptance snapshot, actual current server/assets, Firefox public page '+MODE,flush=True)
finally:
 if proc:
  proc.terminate()
  try:proc.wait(timeout=10)
  except subprocess.TimeoutExpired:proc.kill();proc.wait()
 for name in containers:subprocess.run(['docker','rm','-fv',name],capture_output=True)
