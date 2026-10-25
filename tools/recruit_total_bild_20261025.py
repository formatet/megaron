#!/usr/bin/env python3
"""Recruitment price/shortfall/payment proof: fresh owned PG/Redis/server and native Firefox.
Usage: python3 tools/recruit_total_bild_20261025.py OUTPUT
Fixtures are logged; no shared-world mutations, mocked responses or DOM replacements.
"""
from pathlib import Path
import os,json,secrets,socket,subprocess,tempfile,time,urllib.request,urllib.error,hashlib,sys
from playwright.sync_api import sync_playwright
ROOT=Path(__file__).resolve().parents[1]
OUT=Path(sys.argv[1]).resolve();OUT.mkdir(parents=True,exist_ok=True)
containers=[];proc=None
clean={k:os.environ[k] for k in ('HOME','PATH')};clean['CGO_ENABLED']='0'
def run(*args,**kw):return subprocess.check_output(args,text=True,**kw).strip()
def hashes():return {str(p.relative_to(ROOT)):hashlib.sha256(p.read_bytes()).hexdigest() for d in ('server','web') for p in sorted((ROOT/d).rglob('*')) if p.is_file() and p.suffix in ('.go','.js','.md','.html','.sql','.css','.json') and not p.name.endswith('_test.go')}
before=hashes();fixtures=[];shots=[];observations=[]
(OUT/'source-before.json').write_text(json.dumps(before,indent=2))
def launch(kind,image,args):
 name='megaron-recruit-total-'+secrets.token_hex(4)+'-'+kind;internal='5432' if kind=='pg' else '6379'
 run('docker','run','-d','--rm','--name',name,'-p','127.0.0.1::'+internal,*args,image);containers.append(name)
 return json.loads(run('docker','inspect',name))[0]['NetworkSettings']['Ports'][internal+'/tcp'][0]['HostPort']
try:
 with tempfile.TemporaryDirectory(prefix='recruit-total-proof-') as td:
  tmp=Path(td);commit=run('git','-C',str(ROOT),'rev-parse','--short=8','HEAD')+'+recruit-total'
  for binary,pkg in [('temenos','./cmd/server'),('keryx','./cmd/keryx')]:run('go','build','-ldflags','-X main.buildCommit='+commit,'-o',str(tmp/binary),pkg,cwd=ROOT/'server',env=clean)
  pg=launch('pg','postgres:16-alpine',['-e','POSTGRES_PASSWORD=recruit','-e','POSTGRES_DB=recruit']);redis=launch('redis','redis:7-alpine',[])
  for _ in range(100):
   if subprocess.run(['docker','exec',containers[0],'pg_isready','-U','postgres','-q'],capture_output=True).returncode==0:break
   time.sleep(.25)
  with socket.socket() as s:s.bind(('127.0.0.1',0));port=s.getsockname()[1]
  base=f'http://127.0.0.1:{port}'
  env={**clean,'DATABASE_URL':f'postgres://postgres:recruit@localhost:{pg}/recruit?sslmode=disable','REDIS_URL':'localhost:'+redis,'JWT_SECRET':secrets.token_urlsafe(40),'PORT':str(port),'TICK_SECONDS':'1800','MAP_WIDTH':'56','MAP_HEIGHT':'40','WORLD_NAME':'Tylissos recruitment proof','POLEIA_WORLD_START_WANAXES':'1','STATIC_DIR':str(ROOT/'web/static'),'TEMPLATE_DIR':str(ROOT/'web/templates'),'CHRONICLE_DIR':str(tmp/'chronicles'),'REPORTS_DIR':str(tmp/'reports')}
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
  latest=max(int(p.name.split('_')[0]) for p in (ROOT/'server/db/migrations').glob('*.up.sql'))
  assert health['commit']==commit and health['migration']==latest,health
  world=api('/api/v1/worlds')[0]['id'];wp='/api/v1/worlds/'+world
  token=api('/api/v1/auth/register','POST',{'username':'Recruit-'+secrets.token_hex(4),'password':secrets.token_urlsafe(30)})['access_token']
  api(wp+'/join','POST',{},token)
  def sql(q,fixture=False):
   if fixture:fixtures.append(q);(OUT/'fixture.sql').write_text('\n\n'.join(fixtures)+'\n')
   return run('docker','exec',containers[0],'psql','-U','postgres','-d','recruit','-At','-v','ON_ERROR_STOP=1','-c',q)
  sid=api(wp+'/founding/settle','POST',{'name':'Tylissos'},token)['settlement_id']
  province=next(p['id'] for p in api(wp+'/provinces',token=token) if p.get('settlement_id')==sid)
  sql(f"UPDATE provinces SET coastal=true WHERE id='{province}'; INSERT INTO buildings(settlement_id,building_type,level) VALUES('{sid}','barracks',1),('{sid}','harbour',1),('{sid}','shipyard',1) ON CONFLICT DO NOTHING; UPDATE settlement_goods SET amount=CASE good_key WHEN 'timber' THEN 0 WHEN 'cedar' THEN 749 ELSE 10000 END,rate=0,cap=1000000,calc_tick=current_world_tick() WHERE settlement_id='{sid}';",True)
  cfg=tmp/'keryx.json';cfg.write_text(json.dumps({'server':base,'token':token,'world_id':world,'province_id':province}));cfg.chmod(0o600)
  clienv={**clean,'POLEIA_CONFIG':str(cfg)}
  (OUT/'catalogue.json').write_text(json.dumps(api('/api/v1/units'),indent=2))
  with sync_playwright() as pw:
   browser=pw.firefox.launch();pages=[]
   for width,height in [(1280,900),(390,844)]:
    ctx=browser.new_context(viewport={'width':width,'height':height});ctx.add_cookies([{'name':'poleia_token','value':token,'url':base}]);ctx.add_init_script('localStorage.setItem("poleia_token",'+json.dumps(token)+')')
    page=ctx.new_page();errors=[];page.on('pageerror',lambda e,errors=errors:errors.append(str(e)))
    page.goto(base+'/world/'+world+'/map',wait_until='networkidle')
    if page.locator('#dc-dismiss-all').is_visible():page.locator('#dc-dismiss-all').click()
    for brief in page.locator('.lb-dismiss').all():
     if brief.is_visible():brief.click()
    page.locator('#trig-war').click();page.locator('#war-body [data-tab="recruit"]').click();page.wait_for_selector('#wtab-recruit [data-recruit-type="ship"]')
    pages.append((page,width,errors))
   def phase(code):
    state=api(wp+'/provinces/'+province,token=token);(OUT/(code+'-api.json')).write_text(json.dumps(state,indent=2))
    cli=run(str(tmp/'keryx'),'recruit','--list',env=clienv);(OUT/(code+'-keryx.txt')).write_text(cli+'\n')
    for page,width,errors in pages:
     page.evaluate("async()=>{const m=await import('/static/js/megaron/ui/drawers/war.js');await m.loadWarDrawer();}")
     page.locator('#war-body [data-tab="recruit"]').click();page.wait_for_selector('#wtab-recruit [data-recruit-type="ship"]')
     text=page.locator('#wtab-recruit').inner_text();(OUT/(code+f'-{width}.txt')).write_text(text)
     assert '/man' not in text and 'insufficient' not in text and 'needs 10+ pop' not in text,text
     assert '30 timber (or cedar)' in text and '90 cedar' in text and '20 crew from the city' in text,text
     galley=page.locator('#wtab-recruit [data-recruit-type="ship"]');emporos=page.locator('#wtab-recruit [data-recruit-type="merchantman"]')
     if code=='payable':assert galley.is_enabled() and emporos.is_enabled(),text
     if code=='shortage':assert galley.is_disabled() and 'needs 30 timber (or cedar) (have 10)' in text,text
     galley.scroll_into_view_if_needed();page.screenshot(path=str(OUT/f'{code}-{width}.png'));shots.append({'phase':code,'width':width,'file':f'{code}-{width}.png'})
     assert not errors,errors
    observations.append({'phase':code,'can_recruit':state['settlement']['can_recruit']})
   phase('payable')
   sql(f"UPDATE settlement_goods SET amount=CASE good_key WHEN 'timber' THEN 10 WHEN 'cedar' THEN 0 ELSE amount END,rate=0,calc_tick=current_world_tick() WHERE settlement_id='{sid}';",True)
   phase('shortage')
   sql(f"UPDATE settlement_goods SET amount=CASE good_key WHEN 'timber' THEN 0 WHEN 'cedar' THEN 749 ELSE amount END,rate=0,calc_tick=current_world_tick() WHERE settlement_id='{sid}';",True)
   page=pages[0][0];page.evaluate("async()=>{await (await import('/static/js/megaron/ui/drawers/war.js')).loadWarDrawer()}")
   with page.expect_response(lambda r:r.request.method=='POST' and r.url.endswith('/recruit')) as submitted:page.locator('#wtab-recruit [data-recruit-type="ship"]').click()
   response=submitted.value;assert response.status==201,response.text();receipt=response.json();assert {k:v for k,v in receipt['paid_costs'].items() if v>0}=={'cedar':30},receipt
   (OUT/'actual-recruit.json').write_text(json.dumps(receipt,indent=2))
   for page,width,errors in pages:
    page.wait_for_timeout(150)
    if width==390:page.reload(wait_until='networkidle');page.locator('#trig-war').click();page.locator('#war-body [data-tab="recruit"]').click()
    # Receipts are session-local, so the other viewport reads the same real
    # response via its own real order instead of copying receipt/DOM state.
    if width==390:
     with page.expect_response(lambda r:r.request.method=='POST' and r.url.endswith('/recruit')) as submitted:page.locator('#wtab-recruit [data-recruit-type="ship"]').click()
     second=submitted.value;assert second.status==201,second.text();assert {k:v for k,v in second.json()['paid_costs'].items() if v>0}=={'cedar':30},second.json()
     (OUT/'actual-recruit-mobile.json').write_text(json.dumps(second.json(),indent=2))
    page.wait_for_function("document.querySelector('#war-recruit-res')?.textContent.includes('paid 30 cedar')")
    text=page.locator('#war-recruit-res').inner_text();(OUT/f'paid-{width}.txt').write_text(text);page.locator('#war-recruit-res').scroll_into_view_if_needed();page.screenshot(path=str(OUT/f'paid-{width}.png'));shots.append({'phase':'paid','width':width,'file':f'paid-{width}.png'})
    assert not errors,errors
   browser.close()
  stocks=json.loads(sql(f"SELECT json_object_agg(good_key,amount) FROM settlement_goods WHERE settlement_id='{sid}' AND good_key IN ('timber','cedar')"));assert stocks=={'timber':0,'cedar':689} or stocks=={'cedar':689,'timber':0},stocks
  (OUT/'paid-db.json').write_text(json.dumps(stocks,indent=2))
  assert before==hashes(),'production sources changed during capture'
  (OUT/'proof.json').write_text(json.dumps({'health':health,'source_sha256':before,'source_unchanged':True,'fixture':'Real register/join/found; explicit own Tylissos coastal/shipyard/harbour and goods fixtures. 0 timber +749 cedar pays both Galley and Emporos because cedar substitutes timber. Separate 10 timber +0 cedar phase proves shortage. Real galley recruitment in each viewport pays30 cedar each.','browser_errors':[],'observations':observations,'shots':shots},indent=2))
  print('PASS Firefox1280/390 total costs, named shortfalls, real recruit cedar debit/receipt and API+Keryx; stable sources',flush=True)
finally:
 if proc is not None:proc.terminate();proc.wait(timeout=15)
 for name in containers:subprocess.run(['docker','rm','-fv',name],capture_output=True)
