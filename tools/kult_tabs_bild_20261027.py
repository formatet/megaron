#!/usr/bin/env python3
"""Dispatch actions and truthful omen proof: fresh owned PG/Redis/server and native Firefox.
Usage: python3 tools/dispatch_omen_bild_20261026.py OUTPUT
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
 name='megaron-dispatch-omen-'+secrets.token_hex(4)+'-'+kind;internal='5432' if kind=='pg' else '6379'
 run('docker','run','-d','--rm','--name',name,'-p','127.0.0.1::'+internal,*args,image);containers.append(name)
 return json.loads(run('docker','inspect',name))[0]['NetworkSettings']['Ports'][internal+'/tcp'][0]['HostPort']
try:
 with tempfile.TemporaryDirectory(prefix='dispatch-omen-proof-') as td:
  tmp=Path(td);commit=run('git','-C',str(ROOT),'rev-parse','--short=8','HEAD')+'+kult-tabs'
  for binary,pkg in [('temenos','./cmd/server'),('keryx','./cmd/keryx')]:run('go','build','-ldflags','-X main.buildCommit='+commit,'-o',str(tmp/binary),pkg,cwd=ROOT/'server',env=clean)
  pg=launch('pg','postgres:16-alpine',['-e','POSTGRES_PASSWORD=proof','-e','POSTGRES_DB=proof']);redis=launch('redis','redis:7-alpine',[])
  for _ in range(100):
   if subprocess.run(['docker','exec',containers[0],'pg_isready','-h','127.0.0.1','-U','postgres','-q'],capture_output=True).returncode==0:break
   time.sleep(.25)
  with socket.socket() as s:s.bind(('127.0.0.1',0));port=s.getsockname()[1]
  base=f'http://127.0.0.1:{port}'
  env={**clean,'DATABASE_URL':f'postgres://postgres:proof@localhost:{pg}/proof?sslmode=disable','REDIS_URL':'localhost:'+redis,'JWT_SECRET':secrets.token_urlsafe(40),'PORT':str(port),'TICK_SECONDS':'3600','MAP_WIDTH':'56','MAP_HEIGHT':'40','WORLD_NAME':'Dispatch and omen proof','POLEIA_WORLD_START_WANAXES':'1','STATIC_DIR':str(ROOT/'web/static'),'TEMPLATE_DIR':str(ROOT/'web/templates'),'CHRONICLE_DIR':str(tmp/'chronicles'),'REPORTS_DIR':str(tmp/'reports')}
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
  assert health['commit']==commit and health['migration']==217,health
  world=api('/api/v1/worlds')[0]['id'];wp='/api/v1/worlds/'+world
  token=api('/api/v1/auth/register','POST',{'username':'Dispatch-'+secrets.token_hex(4),'password':secrets.token_urlsafe(30)})['access_token']
  api(wp+'/join','POST',{},token);pid=api('/api/v1/auth/me',token=token)['id']
  def sql(q,fixture=False):
   if fixture:fixtures.append(q);(OUT/'fixture.sql').write_text('\n\n'.join(fixtures)+'\n')
   return run('docker','exec',containers[0],'psql','-U','postgres','-d','proof','-At','-v','ON_ERROR_STOP=1','-c',q)
  host=next(u for u in api(wp+'/units',token=token)['units'] if u['type']=='nomadic_host')
  sql(f"UPDATE map_tiles SET terrain='plains',coastal=false WHERE world_id='{world}';",True)
  home=api(wp+'/founding/settle','POST',{'name':'Tylissos'},token)['settlement_id'];hp=sql(f"SELECT province_id FROM settlements WHERE id='{home}'")
  cities=[{'id':home,'province_id':hp,'name':'Tylissos','god':'sea_daimons_stillness'}]
  for name,q,r in [('Amnissos',10,10),('Kommos',20,20)]:
   prov=sql(f"SELECT id FROM provinces WHERE world_id='{world}' AND map_q={q} AND map_r={r}")
   if not prov:prov=sql(f"INSERT INTO provinces(world_id,map_q,map_r,terrain_type,coastal) VALUES('{world}',{q},{r},'plains',false) RETURNING id;",True).splitlines()[0]
   sid=sql(f"INSERT INTO settlements(world_id,province_id,name,culture_id,owner_id,control_type,is_capital,state,population) VALUES('{world}','{prov}','{name}','minoan','{pid}','colony',false,'active',2000) RETURNING id;",True).splitlines()[0]
   cities.append({'id':sid,'province_id':prov,'name':name,'god':'asterion'})
  for city in cities:
   sid=city['id'];god=city['god']
   sql(f"UPDATE settlements SET culture_id='minoan',population=2000 WHERE id='{sid}'; INSERT INTO buildings(settlement_id,building_type,level,god_id) VALUES('{sid}','temple',1,'{god}') ON CONFLICT DO NOTHING; INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) SELECT '{sid}',key,10000,0,1000000,current_world_tick() FROM goods ON CONFLICT(settlement_id,good_key) DO UPDATE SET amount=10000,rate=0,cap=1000000,calc_tick=current_world_tick();",True)
   api(wp+'/provinces/'+city['province_id']+'/placements','POST',{'target_kind':'building','building_type':'temple','god':god,'good_key':'cult'},token)
  sql(f"UPDATE player_world_records SET kharis_amount=70,kharis_rate=0,kharis_calc_tick=current_world_tick() WHERE player_id='{pid}' AND world_id='{world}';",True)
  cfg=tmp/'keryx.json';cfg.write_text(json.dumps({'server':base,'token':token,'world_id':world,'province_id':hp}));cfg.chmod(0o600);clienv={**clean,'POLEIA_CONFIG':str(cfg)}
  view=api(wp+'/cult',token=token);assert len(view['temples'])==3 and len({t['god_id'] for t in view['temples']})==2,view
  (OUT/'realm.json').write_text(json.dumps(view,indent=2))
  for command in [['status'],['rite','--list']]:
   text=run(str(tmp/'keryx'),*command,env=clienv)
   for city in cities:assert city['name'] in text,(command,city,text)
   if command==['status']:assert text.count('workers 1/2')==3,text
   (OUT/('keryx-'+command[0]+'.txt')).write_text(text)
  with sync_playwright() as pw:
   browser=pw.firefox.launch()
   for cycle in [1,2]:
    for width,height in [(1280,900),(390,844)]:
     ctx=browser.new_context(viewport={'width':width,'height':height});ctx.add_cookies([{'name':'poleia_token','value':token,'url':base}]);ctx.add_init_script('localStorage.setItem("poleia_token",'+json.dumps(token)+')')
     page=ctx.new_page();errors=[];page.on('pageerror',lambda e:errors.append(str(e)));page.goto(base+'/world/'+world+'/map',wait_until='networkidle')
     if page.locator('#dc-dismiss-all').is_visible():page.locator('#dc-dismiss-all').click()
     for brief in page.locator('.lb-dismiss').all():
      if brief.is_visible():brief.click()
     page.locator('#trig-kult').click();page.wait_for_selector('[data-cult-tab="temples"]')
     for tab in ['kharis','prayers','temples']:
      page.locator('[data-cult-tab="'+tab+'"]').click();panel=page.locator('[data-cult-panel="'+tab+'"]');assert panel.is_visible()
      rect=page.locator('#kult-body').evaluate('(e)=>({client:e.clientWidth,scroll:e.scrollWidth})');assert rect['scroll']<=rect['client']+1,rect
      if tab=='temples':
       assert panel.locator('[data-cult-temple]').count()==3
       for city in cities:assert city['name'] in panel.inner_text()
      filename=f'{cycle}-{tab}-{width}.png';page.screenshot(path=str(OUT/filename));shots.append({'file':filename,'tab':tab})
     # Manage opens the correct city's ordinary City surface.
     target=next(t for t in view['temples'] if t['city_name']=='Kommos');page.locator('[data-cult-temple="'+target['id']+'"] [data-cult-city]').click()
     page.wait_for_function('(name)=>document.querySelector("#city-drawer-title")?.textContent.includes(name)',arg=target['city_name'])
     assert not errors,errors
     ctx.close()
   # Real offering from Kommos, not the other city's same-god temple.
   ctx=browser.new_context(viewport={'width':1280,'height':900});ctx.add_cookies([{'name':'poleia_token','value':token,'url':base}]);ctx.add_init_script('localStorage.setItem("poleia_token",'+json.dumps(token)+')')
   page=ctx.new_page();page.goto(base+'/world/'+world+'/map',wait_until='networkidle')
   if page.locator('#dc-dismiss-all').is_visible():page.locator('#dc-dismiss-all').click()
   for brief in page.locator('.lb-dismiss').all():
    if brief.is_visible():brief.click()
   page.locator('#trig-kult').click();page.wait_for_selector('[data-cult-tab="prayers"]');page.locator('[data-cult-tab="prayers"]').click()
   prayer=next(p for p in view['prayers'] if p['city_name']=='Kommos' and p['god_id']=='asterion' and p['can_prepare_offering']);key=prayer['settlement_id']+'__'+prayer['id'];row=page.locator('[data-cult-prayer="'+key+'"]');row.get_by_role('button',name='Offer',exact=True).click();page.wait_for_selector('#offer-row-'+key+' .offer-input')
   before_stock=sql(f"SELECT json_agg(t) FROM(SELECT settlement_id,good_key,amount FROM settlement_goods WHERE settlement_id IN ('{cities[1]['id']}','{cities[2]['id']}') ORDER BY settlement_id,good_key)t")
   with page.expect_response(lambda response: response.request.method=='POST' and response.url.endswith('/settlements/'+prayer['settlement_id']+'/rite')) as sent:
    row.get_by_role('button',name='Bring the offering',exact=True).click()
   assert sent.value.status==200,sent.value.text();(OUT/'rite.json').write_text(json.dumps(sent.value.json(),indent=2));after_stock=sql(f"SELECT json_agg(t) FROM(SELECT settlement_id,good_key,amount FROM settlement_goods WHERE settlement_id IN ('{cities[1]['id']}','{cities[2]['id']}') ORDER BY settlement_id,good_key)t")
   before_rows=json.loads(before_stock);after_rows=json.loads(after_stock);other=cities[1]['id'];assert [r for r in before_rows if r['settlement_id']==other]==[r for r in after_rows if r['settlement_id']==other];assert before_stock!=after_stock
   (OUT/'offering-stock.json').write_text(json.dumps({'before':before_rows,'after':after_rows,'selected_city':prayer['settlement_id']},indent=2))
   # Fresh quote, cancel without DELETE, then confirmed demolition of this city's temple.
   page.locator('[data-cult-tab="temples"]').click();victim=next(t for t in api(wp+'/cult',token=token)['temples'] if t['city_name']=='Kommos')
   row=page.locator('[data-cult-temple="'+victim['id']+'"]');deletes=[];page.on('request',lambda request:deletes.append(request.url) if request.method=='DELETE' else None)
   row.locator('[data-cult-demolish]').click();confirm=page.locator('#cult-demolish-'+victim['id']);confirm.get_by_role('button',name='Keep it',exact=True).click();assert not deletes,deletes
   row.locator('[data-cult-demolish]').click()
   with page.expect_response(lambda response: response.request.method=='DELETE' and response.url.endswith('/buildings/'+victim['id'])) as removed:
    confirm.get_by_role('button',name='Demolish',exact=True).click()
   assert removed.value.status==200,removed.value.text();page.wait_for_selector('[data-cult-temple="'+victim['id']+'"]',state='detached')
   remaining=api(wp+'/cult',token=token)['temples'];assert len(remaining)==2 and not any(t['id']==victim['id'] for t in remaining),remaining
   assert all(t['city_name']!='Kommos' for t in remaining)
   (OUT/'demolition.json').write_text(json.dumps({'response':removed.value.json(),'remaining':remaining,'delete_requests':deletes},indent=2));page.screenshot(path=str(OUT/'demolition-success-1280.png'))
   ctx.close();browser.close()
  assert before==hashes(),'production sources changed during capture'
  (OUT/'proof.json').write_text(json.dumps({'health':health,'source_unchanged':True,'shots':shots,'fixture':'Fresh register/join/found+explicit second/third own cities and three temples/two gods. Actual staffing APIs; native tab/city routing twice1280/390. Actual Kommos offering API+DB confirms other same-god city unchanged; fresh demolition quote, cancel sends no DELETE, confirmed demolition removes only Kommos temple.'},indent=2))
  print('PASS real realm cult three cities/two gods, native tabs/city routing and exact-city offering',flush=True)
finally:
 if proc is not None:proc.terminate();proc.wait(timeout=15)
 for name in containers:subprocess.run(['docker','rm','-fv',name],capture_output=True)
