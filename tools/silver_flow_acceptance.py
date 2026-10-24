#!/usr/bin/env python3
"""Fresh owned runtime; normal register/join/found, explicit own-DB recruit fixture.
Usage: python3 tools/silver_flow_acceptance.py OUTPUT
Never uses shared acceptance or production services.
"""
from pathlib import Path
import os,json,secrets,socket,subprocess,tempfile,time,urllib.request,hashlib,sys
from playwright.sync_api import sync_playwright
ROOT=Path(__file__).resolve().parents[1]
out=Path(sys.argv[1]).resolve();out.mkdir(parents=True,exist_ok=True)
containers=[];proc=None
clean={k:os.environ[k] for k in ('HOME','PATH')};clean['CGO_ENABLED']='0'
def run(*args,**kw):return subprocess.check_output(args,text=True,**kw).strip()
def hashes():return {str(p.relative_to(ROOT)):hashlib.sha256(p.read_bytes()).hexdigest() for d in ('server','web') for p in sorted((ROOT/d).rglob('*')) if p.is_file() and p.suffix in ('.go','.js','.md','.html','.sql') and not p.name.endswith('_test.go')}
before=hashes()
def launch(kind,image,args):
 name='megaron-silverflow-'+secrets.token_hex(4)+'-'+kind;internal='5432' if kind=='pg' else '6379'
 run('docker','run','-d','--rm','--name',name,'-p','127.0.0.1::'+internal,*args,image);containers.append(name)
 return json.loads(run('docker','inspect',name))[0]['NetworkSettings']['Ports'][internal+'/tcp'][0]['HostPort']
try:
 with tempfile.TemporaryDirectory(prefix='silverflow-') as td:
  tmp=Path(td);commit=run('git','-C',str(ROOT),'rev-parse','--short=7','HEAD')+'+silverflow'
  for binary,pkg in [('temenos','./cmd/server'),('keryx','./cmd/keryx')]:run('go','build','-ldflags','-X main.buildCommit='+commit,'-o',str(tmp/binary),pkg,cwd=ROOT/'server',env=clean)
  pg=launch('pg','postgres:16-alpine',['-e','POSTGRES_PASSWORD=silverflow','-e','POSTGRES_DB=silverflow']);redis=launch('redis','redis:7-alpine',[])
  for _ in range(100):
   if subprocess.run(['docker','exec',containers[0],'pg_isready','-U','postgres','-q'],capture_output=True).returncode==0:break
   time.sleep(.25)
  with socket.socket() as s:s.bind(('127.0.0.1',0));port=s.getsockname()[1]
  base=f'http://127.0.0.1:{port}'
  env={**clean,'DATABASE_URL':f'postgres://postgres:silverflow@localhost:{pg}/silverflow?sslmode=disable','REDIS_URL':'localhost:'+redis,'JWT_SECRET':secrets.token_urlsafe(40),'PORT':str(port),'TICK_SECONDS':'3600','MAP_WIDTH':'56','MAP_HEIGHT':'40','WORLD_NAME':'Silver flow proof','POLEIA_WORLD_START_WANAXES':'1','STATIC_DIR':str(ROOT/'web/static'),'TEMPLATE_DIR':str(ROOT/'web/templates'),'CHRONICLE_DIR':str(tmp/'chronicles'),'REPORTS_DIR':str(tmp/'reports')}
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
  fixture=f"UPDATE provinces SET coastal=true WHERE id='{province}'; INSERT INTO buildings(settlement_id,building_type,level) VALUES('{sid}','barracks',1),('{sid}','harbour',1),('{sid}','shipyard',1) ON CONFLICT DO NOTHING; UPDATE settlement_goods SET amount=CASE good_key WHEN 'grain' THEN 1000 WHEN 'timber' THEN 300 WHEN 'cedar' THEN 160 ELSE 0 END,rate=0,cap=1000000,calc_tick=current_world_tick() WHERE settlement_id='{sid}';"
  (out/'fixture.sql').write_text(fixture);sql(fixture)
  cfg=tmp/'config.json';cfg.write_text(json.dumps({'server':base,'world_id':world,'province_id':province,'token':token}));cfg.chmod(0o600);clienv={**clean,'POLEIA_CONFIG':str(cfg)}
  catalogue=api('/api/v1/units',token=token);(out/'catalogue.json').write_text(json.dumps(catalogue,indent=2))
  types={u['type']:u for u in catalogue}
  for kind,cost in [('spearman',{'grain':12}),('galley',{'timber':30}),('merchantman',{'cedar':16})]:assert types[kind]['costs']==cost,(kind,types[kind])
  cli=run(str(tmp/'keryx'),'recruit','--list',env=clienv);(out/'keryx-recruit.txt').write_text(cli)
  for key in ('cedar','16','30'):assert key in cli,cli
  with sync_playwright() as pw:
   browser=pw.firefox.launch()
   for label,width,height in [('desktop',1280,900),('mobile',390,844)]:
    ctx=browser.new_context(viewport={'width':width,'height':height});ctx.add_cookies([{'name':'poleia_token','value':token,'url':base}]);ctx.add_init_script('localStorage.setItem("poleia_token",'+json.dumps(token)+')')
    page=ctx.new_page();errors=[];page.on('pageerror',lambda e:errors.append(str(e)))
    page.goto(base+'/world/'+world+'/map',wait_until='networkidle')
    if page.locator('#dc-dismiss-all').is_visible():page.locator('#dc-dismiss-all').click()
    for brief in page.locator('.drawer.open .lb-dismiss').all():
     if brief.is_visible():brief.click()
    page.locator('#trig-war').click()
    page.locator('#drawer-war [data-tab="recruit"]').click()
    page.wait_for_timeout(400)
    text=page.locator('#drawer-war').inner_text();(out/(label+'-recruit.txt')).write_text(text)
    assert '9.0 cedar/man' in text and '1.5 timber/man' in text,text
    page.screenshot(path=str(out/(label+'.png')))
    # Real browser action: train a cohort against zero silver.
    with page.expect_response(lambda response: response.request.method=='POST' and response.url.endswith('/recruit')) as submitted:
     page.locator('[data-recruit-type="spearman"]').click()
    assert submitted.value.status==201,submitted.value.text()
    page.wait_for_timeout(500)
    assert not errors,errors
    ctx.close()
   browser.close()
  # Actual CLI recruitment uses the same zero-silver city.
  cli_recruit=run(str(tmp/'keryx'),'recruit','--unit','merchantman',env=clienv)
  (out/'keryx-emporos.txt').write_text(cli_recruit)
  assert float(sql(f"SELECT amount FROM settlement_goods WHERE settlement_id='{sid}' AND good_key='silver'"))==0
  assert float(sql(f"SELECT amount FROM settlement_goods WHERE settlement_id='{sid}' AND good_key='cedar'"))==144
  assert before==hashes(),'source changed during acceptance'
  (out/'proof.json').write_text(json.dumps({'health':health,'host_silver':purse,'source_sha256':before,'browser_errors':[],'cli_recruited_emporos':True},indent=2))
  print('PASS normal join/found120, live catalogue, Firefox1280/390 recruits with0silver, actual CLI Emporos16cedar, source stable',flush=True)
finally:
 if proc is not None:proc.terminate();proc.wait(timeout=15)
 for name in containers:subprocess.run(['docker','rm','-fv',name],capture_output=True)
