#!/usr/bin/env python3
"""Game-day wording and actual-cadence proof, with an owned fresh runtime.
Usage: python3 tools/game_days_acceptance.py OUTPUT [TICK_SECONDS]
Never uses shared acceptance or production services.
"""
from pathlib import Path
import os,json,secrets,socket,subprocess,tempfile,time,urllib.request,hashlib,sys
from playwright.sync_api import sync_playwright
ROOT=Path(__file__).resolve().parents[1]
out=Path(sys.argv[1]).resolve();out.mkdir(parents=True,exist_ok=True)
cadence=int(sys.argv[2]) if len(sys.argv)>2 else 1800
containers=[];proc=None
clean={k:os.environ[k] for k in ('HOME','PATH')};clean['CGO_ENABLED']='0'
def run(*args,**kw):return subprocess.check_output(args,text=True,**kw).strip()
def hashes():return {str(p.relative_to(ROOT)):hashlib.sha256(p.read_bytes()).hexdigest() for d in ('server','web') for p in sorted((ROOT/d).rglob('*')) if p.is_file() and p.suffix in ('.go','.js','.md','.html','.sql') and not p.name.endswith('_test.go')}
before=hashes()
def launch(kind,image,args):
 name='megaron-gamedays-'+secrets.token_hex(4)+'-'+kind;internal='5432' if kind=='pg' else '6379'
 run('docker','run','-d','--rm','--name',name,'-p','127.0.0.1::'+internal,*args,image);containers.append(name)
 return json.loads(run('docker','inspect',name))[0]['NetworkSettings']['Ports'][internal+'/tcp'][0]['HostPort']
try:
 with tempfile.TemporaryDirectory(prefix='silverflow-') as td:
  tmp=Path(td);commit=run('git','-C',str(ROOT),'rev-parse','--short=7','HEAD')+'+game-days'
  for binary,pkg in [('temenos','./cmd/server'),('keryx','./cmd/keryx')]:run('go','build','-ldflags','-X main.buildCommit='+commit,'-o',str(tmp/binary),pkg,cwd=ROOT/'server',env=clean)
  pg=launch('pg','postgres:16-alpine',['-e','POSTGRES_PASSWORD=silverflow','-e','POSTGRES_DB=silverflow']);redis=launch('redis','redis:7-alpine',[])
  for _ in range(100):
   if subprocess.run(['docker','exec',containers[0],'pg_isready','-U','postgres','-q'],capture_output=True).returncode==0:break
   time.sleep(.25)
  with socket.socket() as s:s.bind(('127.0.0.1',0));port=s.getsockname()[1]
  base=f'http://127.0.0.1:{port}'
  env={**clean,'DATABASE_URL':f'postgres://postgres:silverflow@localhost:{pg}/silverflow?sslmode=disable','REDIS_URL':'localhost:'+redis,'JWT_SECRET':secrets.token_urlsafe(40),'PORT':str(port),'TICK_SECONDS':str(cadence),'MAP_WIDTH':'56','MAP_HEIGHT':'40','WORLD_NAME':'Game days proof','POLEIA_WORLD_START_WANAXES':'1','STATIC_DIR':str(ROOT/'web/static'),'TEMPLATE_DIR':str(ROOT/'web/templates'),'CHRONICLE_DIR':str(tmp/'chronicles'),'REPORTS_DIR':str(tmp/'reports')}
  assert not (ROOT/'server/.env').exists()
  proc=subprocess.Popen([str(tmp/'temenos')],cwd=ROOT/'server',env=env,stdout=(out/'server.log').open('w'),stderr=subprocess.STDOUT)
  def api(path,method='GET',data=None,token=None):
   req=urllib.request.Request(base+path,method=method,data=None if data is None else json.dumps(data).encode(),headers={'Content-Type':'application/json',**({'Authorization':'Bearer '+token} if token else {})})
   return json.load(urllib.request.urlopen(req,timeout=30))
  for _ in range(120):
   try:health=api('/healthz');break
   except Exception:
    if proc.poll() is not None:raise RuntimeError('server exited')
    time.sleep(.5)
  assert health['commit']==commit and health['migration']==210,health
  world=api('/api/v1/worlds')[0]['id'];wp='/api/v1/worlds/'+world
  token=api('/api/v1/auth/register','POST',{'username':'SilverFlow-'+secrets.token_hex(4),'password':secrets.token_urlsafe(30)})['access_token']
  api(wp+'/join','POST',{},token)
  def sql(q):return run('docker','exec',containers[0],'psql','-U','postgres','-d','silverflow','-At','-v','ON_ERROR_STOP=1','-c',q)
  purse=float(sql(f"SELECT silver_amount FROM founder_phase WHERE world_id='{world}' AND active"));assert purse==120,purse
  sid=api(wp+'/founding/settle','POST',{'name':'Silverflow'},token)['settlement_id']
  province=next(p['id'] for p in api(wp+'/provinces',token=token) if p.get('settlement_id')==sid)
  assert float(sql(f"SELECT amount FROM settlement_goods WHERE settlement_id='{sid}' AND good_key='silver'"))==120
  fixture=f"UPDATE provinces SET coastal=true WHERE id='{province}'; INSERT INTO buildings(settlement_id,building_type,level) VALUES('{sid}','barracks',1),('{sid}','harbour',1),('{sid}','shipyard',1) ON CONFLICT DO NOTHING; UPDATE settlement_goods SET amount=CASE good_key WHEN 'grain' THEN 1000 WHEN 'timber' THEN 300 WHEN 'cedar' THEN 10000 ELSE 10000 END,rate=0,cap=1000000,calc_tick=current_world_tick() WHERE settlement_id='{sid}';"
  (out/'fixture.sql').write_text(fixture);sql(fixture)
  cfg=tmp/'config.json';cfg.write_text(json.dumps({'server':base,'world_id':world,'province_id':province,'token':token}));cfg.chmod(0o600);clienv={**clean,'POLEIA_CONFIG':str(cfg)}
  catalogue=api('/api/v1/units',token=token);(out/'catalogue.json').write_text(json.dumps(catalogue,indent=2))
  types={u['type']:u for u in catalogue}
  for kind,cost in [('spearman',{'grain':12}),('galley',{'timber':30}),('merchantman',{'cedar':16})]:assert types[kind]['costs']==cost,(kind,types[kind])
  cli=run(str(tmp/'keryx'),'recruit','--list',env=clienv);(out/'keryx-recruit.txt').write_text(cli)
  for key in ('cedar','16','30'):assert key in cli,cli
  assert 'per game day' in cli and 'game days' in cli,cli
  alpha=urllib.request.urlopen(base+'/alfatestinfo').read().decode()
  expected='30 minutes' if cadence==1800 else '1 hour' if cadence==3600 else str(cadence)+' seconds'
  assert 'A game day passes every '+expected in alpha,expected
  with sync_playwright() as pw:
   browser=pw.firefox.launch()
   for label,width,height in [('desktop',1280,900),('mobile',390,844)]:
    ctx=browser.new_context(viewport={'width':width,'height':height});ctx.add_cookies([{'name':'poleia_token','value':token,'url':base}]);ctx.add_init_script('localStorage.setItem("poleia_token",'+json.dumps(token)+')')
    page=ctx.new_page();errors=[];page.on('pageerror',lambda e:errors.append(str(e)))
    page.goto(base+'/world/'+world+'/map',wait_until='networkidle')
    if page.locator('#dc-dismiss-all').is_visible():page.locator('#dc-dismiss-all').click()
    for brief in page.locator('.drawer.open .lb-dismiss').all():
     if brief.is_visible():brief.click()
    page.locator('#trig-city').click()
    page.locator('#drawer-city [data-tab="byggnader"]').click()
    page.locator('#city-build-select').select_option('stable')
    page.wait_for_timeout(400)
    quote=page.locator('#city-build-quote').inner_text()
    assert 'game days' in quote and 'about ' in quote and 'hours' in quote and 'if started now, ready' in quote,quote
    import re
    duration=re.search(r'([0-9,.]+) game days · about ([0-9,.]+) hours',quote)
    assert duration and abs(float(duration[1].replace(',',''))*cadence/3600-float(duration[2].replace(',','')))<.011,quote
    # The actual runtime State cadence must agree with the public world API.
    assert page.evaluate("async()=> (await import('/static/js/megaron/state.js')).State.TICK_SECONDS")==cadence
    (out/(label+'-quote.txt')).write_text(quote)
    page.locator('#city-build-quote').scroll_into_view_if_needed()
    page.screenshot(path=str(out/(label+'.png')))
    if label=='mobile':
     with page.expect_response(lambda response: response.request.method=='POST' and response.url.endswith('/build')) as submitted:
      page.locator('#city-bld-sec button[onclick="startBuild()"]').click()
     response=submitted.value;assert response.status==201,response.text()
     receipt=response.json();assert 'complete_tick' in receipt and 'duration_game_days' in receipt,receipt
     page.wait_for_timeout(400)
     text=page.locator('#city-build-result').inner_text();assert 'game days' in text and 'about ' in text and 'ready ' in text,text
     (out/'build-receipt.json').write_text(json.dumps(receipt,indent=2));(out/'build-receipt.txt').write_text(text)
    assert not errors,errors
    ctx.close()
   browser.close()
  cli_build=run(str(tmp/'keryx'),'build','--type','pen',env=clienv)
  assert 'game days' in cli_build and 'about ' in cli_build and 'ready ' in cli_build,cli_build
  (out/'keryx-build.txt').write_text(cli_build)
  assert before==hashes(),'source changed during acceptance'
  (out/'proof.json').write_text(json.dumps({'health':health,'tick_seconds':cadence,'host_silver':purse,'source_sha256':before,'browser_errors':[],'cli_build':cli_build},indent=2))
  print('PASS runtimecadence'+str(cadence)+', normal join/found, Firefox1280/390 quotes+actual build receipt, actual CLI build, source stable',flush=True)

finally:
 if proc is not None:proc.terminate();proc.wait(timeout=15)
 for name in containers:subprocess.run(['docker','rm','-fv',name],capture_output=True)
