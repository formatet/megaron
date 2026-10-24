#!/usr/bin/env python3
"""Freight manifest proof: fresh owned rig, real API and Firefox1280/390.
Usage: python3 tools/flervaror_last_bild_20261024.py OUTPUT
Explicit scene fixtures are logged. No shared acceptance or production state.
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
def launch(kind,image,args):
 name='megaron-cargo-'+secrets.token_hex(4)+'-'+kind;internal='5432' if kind=='pg' else '6379'
 run('docker','run','-d','--rm','--name',name,'-p','127.0.0.1::'+internal,*args,image);containers.append(name)
 return json.loads(run('docker','inspect',name))[0]['NetworkSettings']['Ports'][internal+'/tcp'][0]['HostPort']
try:
 with tempfile.TemporaryDirectory(prefix='bildplan-proof-') as td:
  tmp=Path(td);commit=run('git','-C',str(ROOT),'rev-parse','--short=8','HEAD')
  run('go','build','-ldflags','-X main.buildCommit='+commit,'-o',str(tmp/'temenos'),'./cmd/server',cwd=ROOT/'server',env=clean)
  run('go','build','-o',str(tmp/'keryx'),'./cmd/keryx',cwd=ROOT/'server',env=clean)
  pg=launch('pg','postgres:16-alpine',['-e','POSTGRES_PASSWORD=bildplan','-e','POSTGRES_DB=bildplan']);redis=launch('redis','redis:7-alpine',[])
  for _ in range(100):
   if subprocess.run(['docker','exec',containers[0],'pg_isready','-U','postgres','-q'],capture_output=True).returncode==0:break
   time.sleep(.25)
  with socket.socket() as s:s.bind(('127.0.0.1',0));port=s.getsockname()[1]
  base=f'http://127.0.0.1:{port}'
  env={**clean,'DATABASE_URL':f'postgres://postgres:bildplan@localhost:{pg}/bildplan?sslmode=disable','REDIS_URL':'localhost:'+redis,'JWT_SECRET':secrets.token_urlsafe(40),'PORT':str(port),'TICK_SECONDS':'1800','MAP_WIDTH':'56','MAP_HEIGHT':'40','WORLD_NAME':'Lato visual proof','POLEIA_WORLD_START_WANAXES':'1','STATIC_DIR':str(ROOT/'web/static'),'TEMPLATE_DIR':str(ROOT/'web/templates'),'CHRONICLE_DIR':str(tmp/'chronicles'),'REPORTS_DIR':str(tmp/'reports')}
  assert not (ROOT/'server/.env').exists()
  proc=subprocess.Popen([str(tmp/'temenos')],cwd=ROOT/'server',env=env,stdout=(OUT/'server.log').open('w'),stderr=subprocess.STDOUT)
  def api(path,method='GET',data=None,token=None):
   req=urllib.request.Request(base+path,method=method,data=None if data is None else json.dumps(data).encode(),headers={'Content-Type':'application/json',**({'Authorization':'Bearer '+token} if token else {})})
   return json.load(urllib.request.urlopen(req,timeout=30))
  for _ in range(120):
   try:health=api('/healthz');break
   except Exception:
    if proc.poll() is not None:raise RuntimeError('server exited')
    time.sleep(.5)
  assert health['commit']==commit and health['migration']==211,health
  world=api('/api/v1/worlds')[0]['id'];wp='/api/v1/worlds/'+world
  def sql(q,fixture=False):
   if fixture:fixtures.append(q);(OUT/'fixture.sql').write_text('\n\n'.join(fixtures)+'\n')
   return run('docker','exec',containers[0],'psql','-U','postgres','-d','bildplan','-At','-v','ON_ERROR_STOP=1','-c',q)
  def actor(name):
   token=api('/api/v1/auth/register','POST',{'username':name+'-'+secrets.token_hex(3),'password':secrets.token_urlsafe(30)})['access_token']
   api(wp+'/join','POST',{},token);pid=api('/api/v1/auth/me',token=token)['id']
   host=next(u for u in api(wp+'/units',token=token)['units'] if u['type']=='nomadic_host')
   sql(f"UPDATE players SET wanax_name='{name}' WHERE id='{pid}'",True)
   return dict(name=name,token=token,pid=pid,host=host['id'])
  me=actor('Leandros');other=actor('Myrine')
  site=json.loads(sql(f"SELECT row_to_json(t) FROM(SELECT q,r FROM map_tiles WHERE world_id='{world}' AND terrain='plains' AND q>8 AND q<40 AND r>8 AND r<30 ORDER BY q,r LIMIT 1)t"));q,r=site['q'],site['r']
  sql(f"UPDATE map_tiles SET terrain='plains',coastal=true WHERE world_id='{world}' AND r={r} AND q BETWEEN {q} AND {q+6}; UPDATE map_tiles SET terrain='coastal_sea' WHERE world_id='{world}' AND r={r+1} AND q BETWEEN {q-1} AND {q+7};",True)
  for who,x,name in [(me,q,'Lato'),(other,q+6,'Gournia')]:
   sql(f"UPDATE units SET q={x},r={r},status='positioned' WHERE id='{who['host']}';",True)
   who['sid']=api(wp+'/founding/settle','POST',{'name':name},who['token'])['settlement_id']
   who['province']=next(p['id'] for p in api(wp+'/provinces',token=who['token']) if p.get('settlement_id')==who['sid'])
  sql(f"UPDATE worlds SET current_tick=100,last_tick_at=now() WHERE id='{world}'; UPDATE provinces SET coastal=true WHERE id IN('{me['province']}','{other['province']}'); UPDATE settlements SET owner_id='{me['pid']}' WHERE id='{other['sid']}'; UPDATE units SET status='disbanded' WHERE world_id='{world}' AND owner_id='{other['pid']}'; UPDATE units SET q={q},r={r},status='garrison',settlement_id='{me['sid']}',support_settlement_id='{me['sid']}' WHERE world_id='{world}' AND owner_id='{me['pid']}' AND type='spearman'; UPDATE settlement_goods SET amount=10000,rate=0,cap=1000000,calc_tick=100 WHERE settlement_id IN('{me['sid']}','{other['sid']}'); INSERT INTO buildings(settlement_id,building_type,level) VALUES('{me['sid']}','harbour',1),('{other['sid']}','harbour',1) ON CONFLICT DO NOTHING; DELETE FROM sea_storms WHERE world_id='{world}';",True)
  sql(f"UPDATE units SET status='disbanded' WHERE world_id='{world}' AND category='naval';",True)
  ship=sql(f"INSERT INTO units(world_id,owner_id,type,category,size,crew,status,settlement_id,support_settlement_id,q,r,name,hull) VALUES('{world}','{me['pid']}','galley','naval',1,20,'garrison','{me['sid']}','{me['sid']}',{q},{r},'White Dolphin',5) RETURNING id",True).splitlines()[0]
  preview=api(wp+'/provinces/'+me['province']+'/trade/preview?destination_id='+other['sid'],token=me['token'])
  assert preview['category']=='naval' and preview['cargo_capacity']==20,preview
  (OUT/'preview.json').write_text(json.dumps(preview,indent=2))
  print('RIG READY '+base+' health='+json.dumps(health),flush=True)
  with sync_playwright() as pw:
   browser=pw.firefox.launch()
   for width,height in [(1280,900),(390,844)]:
    if width==390:
     ship=sql(f"INSERT INTO units(world_id,owner_id,type,category,size,crew,status,settlement_id,support_settlement_id,q,r,name,hull) VALUES('{world}','{me['pid']}','galley','naval',1,20,'garrison','{me['sid']}','{me['sid']}',{q},{r},'Blue Heron',5) RETURNING id",True).splitlines()[0]
    ctx=browser.new_context(viewport={'width':width,'height':height})
    ctx.add_cookies([{'name':'poleia_token','value':me['token'],'url':base}]);ctx.add_init_script('localStorage.setItem("poleia_token",'+json.dumps(me['token'])+')')
    page=ctx.new_page();errors=[];page.on('pageerror',lambda e:errors.append(str(e)))
    page.goto(base+'/world/'+world+'/map',wait_until='networkidle')
    if page.locator('#dc-dismiss-all').is_visible():page.locator('#dc-dismiss-all').click()
    for brief in page.locator('.drawer.open .lb-dismiss').all():
     if brief.is_visible():brief.click()
    page.evaluate("()=>window.openDrawer('economy')")
    page.locator('[data-tab="transfer"]').click()
    if width==390:
     page.wait_for_function("document.querySelector('#ec-tr-cargo').textContent.includes('timber')")
    page.locator('#ec-tr-from').select_option(me['province'])
    page.locator('#ec-tr-from').dispatch_event('change')
    page.wait_for_function("document.querySelector('#ec-tr-capacity').textContent.includes('/ 20')")
    page.locator('#ec-tr-items select').select_option('grain')
    page.locator('#ec-tr-add').click()
    page.locator('#ec-tr-items select').nth(1).select_option('timber')
    grain=page.locator('#ec-tr-items input[data-good="grain"]');timber=page.locator('#ec-tr-items input[data-good="timber"]')
    for code,values in [('E',(0,0)),('P',(12,6)),('O',(12,9))]:
     grain.fill(str(values[0]));timber.fill(str(values[1]));page.wait_for_timeout(100)
     if code=='O':assert page.locator('#ec-tr-send').is_disabled()
     else:assert not page.locator('#ec-tr-send').is_disabled()
     page.locator('#ec-tr-capacity').scroll_into_view_if_needed()
     filename=f'{code}-{width}.png';page.screenshot(path=str(OUT/filename));shots.append({'code':code+str(width),'file':filename,'width':width,'caption':page.locator('#ec-tr-capacity').inner_text()})
    timber.fill('8')
    with page.expect_response(lambda r:r.request.method=='POST' and r.url.endswith('/trade')) as response:
     page.locator('#ec-tr-send').click()
    result=response.value.json();assert response.value.status==201,result
    assert len(result['items'])==2 and result['cargo_quantity']==20,result
    (OUT/f'dispatch-{width}.json').write_text(json.dumps(result,indent=2))
    page.wait_for_function("document.querySelector('#ec-tr-result').textContent.includes('12 grain')")
    assert '8 timber' in page.locator('#ec-tr-result').inner_text()
    page.wait_for_function("document.querySelector('#ec-tr-cargo').textContent.includes('timber')")
    page.locator('#ec-tr-cargo').scroll_into_view_if_needed()
    filename=f'T-{width}.png';page.screenshot(path=str(OUT/filename));shots.append({'code':'T'+str(width),'file':filename,'width':width,'caption':'Actual dispatch: one ship, 12 grain + 8 timber; both goods in transit'})
    assert sql(f"SELECT count(*) FROM transports WHERE id='{result['transport_id']}' AND ship_unit_id='{ship}'")=='1'
    assert sql(f"SELECT count(*) FROM transport_goods WHERE transport_id='{result['transport_id']}'")=='2'
    assert not errors,errors
    observations.append({'width':width,'page_errors':errors,'result':result})
    ctx.close()
   browser.close()
  # Actual CLI against the same owned rig, with an independent free Emporos.
  sql(f"INSERT INTO units(world_id,owner_id,type,category,size,crew,status,settlement_id,support_settlement_id,q,r,name,hull) VALUES('{world}','{me['pid']}','merchantman','naval',1,10,'garrison','{me['sid']}','{me['sid']}',{q},{r},'Amber Bird',5)",True)
  config=tmp/'keryx.json';config.write_text(json.dumps({'server':base,'token':me['token'],'world_id':world,'province_id':me['province']}))
  cli=run(str(tmp/'keryx'),'transfer','--dest',other['sid'],'--good','grain','--qty','12','--good','timber','--qty','8',env={**clean,'POLEIA_CONFIG':str(config)})
  assert 'Cargo 20 / 120' in cli and '12 grain, 8 timber' in cli,cli
  (OUT/'keryx-transfer.txt').write_text(cli+'\n')
  cargo=run(str(tmp/'keryx'),'cargo',env={**clean,'POLEIA_CONFIG':str(config)})
  assert 'grain' in cargo and 'timber' in cargo,cargo
  (OUT/'keryx-cargo.txt').write_text(cargo+'\n')
  (OUT/'proof.json').write_text(json.dumps({'health':health,'fixture':'Actual app form/API against fresh owned PG/Redis/server. Explicit same-owner coastal cities with free Galley, stocks10000, tick100; a second fresh Galley supplied for mobile, desktop shipment stays in transit. No mocked network/DOM.','source_sha256':before,'source_unchanged':before==hashes(),'observations':observations,'shots':shots},indent=2))
  print('PASS Firefox cargo meter1280/390 empty/partial/overload+actual multi POST',flush=True)
finally:
 if proc is not None:proc.terminate();proc.wait(timeout=15)
 for name in containers:subprocess.run(['docker','rm','-fv',name],capture_output=True)
