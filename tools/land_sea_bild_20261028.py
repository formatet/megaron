#!/usr/bin/env python3
"""Land and sea transport choice proof: fresh owned PG/Redis/server and native Firefox.
Usage: python3 tools/land_sea_bild_20261028.py OUTPUT
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
 name='megaron-land-sea-'+secrets.token_hex(4)+'-'+kind;internal='5432' if kind=='pg' else '6379'
 run('docker','run','-d','--rm','--name',name,'-p','127.0.0.1::'+internal,*args,image);containers.append(name)
 return json.loads(run('docker','inspect',name))[0]['NetworkSettings']['Ports'][internal+'/tcp'][0]['HostPort']
try:
 with tempfile.TemporaryDirectory(prefix='land-sea-proof-') as td:
  tmp=Path(td);commit=run('git','-C',str(ROOT),'rev-parse','--short=8','HEAD')+'+land-sea'
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
  assert health['commit']==commit and health['migration']==218,health
  world=api('/api/v1/worlds')[0]['id'];wp='/api/v1/worlds/'+world
  token=api('/api/v1/auth/register','POST',{'username':'Dispatch-'+secrets.token_hex(4),'password':secrets.token_urlsafe(30)})['access_token']
  api(wp+'/join','POST',{},token);pid=api('/api/v1/auth/me',token=token)['id']
  def sql(q,fixture=False):
   if fixture:fixtures.append(q);(OUT/'fixture.sql').write_text('\n\n'.join(fixtures)+'\n')
   return run('docker','exec',containers[0],'psql','-U','postgres','-d','proof','-At','-v','ON_ERROR_STOP=1','-c',q)
  host=next(u for u in api(wp+'/units',token=token)['units'] if u['type']=='nomadic_host')
  site=json.loads(sql(f"SELECT row_to_json(t) FROM(SELECT q,r FROM map_tiles mt WHERE world_id='{world}' AND q BETWEEN 5 AND 30 AND r BETWEEN 7 AND 30 AND NOT EXISTS(SELECT 1 FROM provinces p WHERE p.world_id=mt.world_id AND ((abs(p.map_q-mt.q)<=3 AND abs(p.map_r-mt.r)<=3) OR (p.map_q IN(mt.q+5,mt.q+13) AND p.map_r=mt.r))) ORDER BY q,r LIMIT 1)t"));hq,hr=site['q'],site['r']
  # Explicit island/port fixture on unoccupied ground; orders below use normal APIs.
  sql(f"UPDATE map_tiles SET terrain='coastal_sea',coastal=false WHERE world_id='{world}'; UPDATE map_tiles SET terrain='plains' WHERE world_id='{world}' AND q BETWEEN {hq} AND {hq+6} AND r BETWEEN {hr} AND {hr+3}; UPDATE map_tiles SET terrain='plains',coastal=true WHERE world_id='{world}' AND q={hq+13} AND r={hr}; UPDATE map_tiles SET coastal=true WHERE world_id='{world}' AND q={hq} AND r={hr}; UPDATE units SET q={hq},r={hr},status='positioned' WHERE id='{host['id']}'; DELETE FROM sea_storms WHERE world_id='{world}';",True)
  for kind in ['elite_infantry','war_chariot']:
   sql(f"INSERT INTO units(world_id,owner_id,type,category,size,ordinal,status,q,r) VALUES('{world}','{pid}','{kind}','land',100,1,'positioned',{hq},{hr});",True)
  for kind,name in [('merchantman','Cargo '+str(i)) for i in range(10)]:
   sql(f"INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('{world}','{pid}','{kind}','naval',1,10,'{name}','positioned',{hq-1},{hr});",True)
  config=tmp/'keryx.json';config.write_text(json.dumps({'server':base,'token':token,'world_id':world}));config.chmod(0o600);clienv={**clean,'POLEIA_CONFIG':str(config)}
  def browser_page(browser,width,height):
   ctx=browser.new_context(viewport={'width':width,'height':height});ctx.add_cookies([{'name':'poleia_token','value':token,'url':base}]);ctx.add_init_script('localStorage.setItem("poleia_token",'+json.dumps(token)+')')
   page=ctx.new_page();errors=[];page.on('pageerror',lambda error:errors.append(str(error)));page.goto(base+'/world/'+world+'/map',wait_until='networkidle')
   if page.locator('#dc-dismiss-all').is_visible():page.locator('#dc-dismiss-all').click()
   for brief in page.locator('.lb-dismiss').all():
    if brief.is_visible():brief.click()
   return ctx,page,errors
  def shot(page,code):
   page.wait_for_timeout(150);rect=page.locator('.drawer.open .drawer-body').evaluate('(e)=>({client:e.clientWidth,scroll:e.scrollWidth})');assert rect['scroll']<=rect['client']+1,rect
   page.screenshot(path=str(OUT/(code+'.png')));shots.append({'file':code+'.png'})
  home=api(wp+'/founding/settle','POST',{'name':'Lato'},token)['settlement_id'];hp=sql(f"SELECT province_id FROM settlements WHERE id='{home}'")
  sql(f"UPDATE provinces SET coastal=true WHERE id='{hp}'; UPDATE settlements SET population=4000 WHERE id='{home}'; INSERT INTO buildings(settlement_id,building_type,level) VALUES('{home}','harbour',1) ON CONFLICT DO NOTHING; UPDATE units SET settlement_id='{home}',support_settlement_id='{home}',status='garrison',q=NULL,r=NULL WHERE world_id='{world}' AND owner_id='{pid}' AND type<>'nomadic_host' AND status<>'disbanded';",True)
  cities=[]
  for name,q,r,coastal in [('Petras',hq+5,hr,True),('Amnissos',hq+13,hr,True),('Inland',hq+2,hr+3,False),('Kyme',hq+5,hr+3,True)]:
   prov=sql(f"SELECT id FROM provinces WHERE world_id='{world}' AND map_q={q} AND map_r={r}")
   if not prov:prov=sql(f"INSERT INTO provinces(world_id,map_q,map_r,terrain_type,coastal) VALUES('{world}',{q},{r},'plains',{str(coastal).lower()}) RETURNING id;",True).splitlines()[0]
   sql(f"UPDATE provinces SET terrain_type='plains',coastal={str(coastal).lower()} WHERE id='{prov}';",True)
   sid=sql(f"INSERT INTO settlements(world_id,province_id,name,culture_id,owner_id,control_type,state,population) VALUES('{world}','{prov}','{name}','minoan','{pid}','colony','active',2000) RETURNING id;",True).splitlines()[0]
   if coastal:sql(f"INSERT INTO buildings(settlement_id,building_type,level) VALUES('{sid}','harbour',1);",True)
   cities.append({'id':sid,'province_id':prov,'name':name})
  allcities=[{'id':home,'province_id':hp,'name':'Lato'},*cities]
  for city in allcities:
   sid=city['id'];sql(f"INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) SELECT '{sid}',key,10000,0,1000000,current_world_tick() FROM goods ON CONFLICT(settlement_id,good_key) DO UPDATE SET amount=10000,rate=0,cap=1000000,calc_tick=current_world_tick();",True)
  # A separately registered neighbour, explicitly contacted in this fresh fixture.
  other_token=api('/api/v1/auth/register','POST',{'username':'Neighbour-'+secrets.token_hex(4),'password':secrets.token_urlsafe(30)})['access_token']
  other=api('/api/v1/auth/me',token=other_token)['id']
  foreign=cities[3]
  sql(f"UPDATE settlements SET owner_id='{other}' WHERE id='{foreign['id']}'; INSERT INTO player_scouted_tiles(world_id,player_id,q,r) VALUES('{world}','{pid}',{hq+5},{hr+3}) ON CONFLICT DO NOTHING;",True)
  options=api(wp+'/settlements/'+home+'/transport-options?destination_id='+cities[0]['id'],token=token)
  assert {o['by'] for o in options['options'] if o['available']}=={'land','sea'},options
  isolated=api(wp+'/settlements/'+home+'/transport-options?destination_id='+cities[1]['id'],token=token)
  assert any(o['by']=='land' and not o['available'] and o['reason'] for o in isolated['options']),isolated
  (OUT/'options.json').write_text(json.dumps({'both':options,'sea_only':isolated},indent=2))
  with sync_playwright() as pw:
   browser=pw.firefox.launch()
   for cycle in [1,2]:
    for width,height in [(1280,900),(390,844)]:
     ctx,page,errors=browser_page(browser,width,height)
     page.locator('#trig-economy').click();page.locator('#economy-body [data-tab="transfer"]').click()
     page.locator('#ec-tr-to').select_option(cities[0]['id']);page.wait_for_selector('#ec-tr-route input[value="land"]')
     assert page.locator('#ec-tr-route input[value="land"]').is_enabled() and page.locator('#ec-tr-route input[value="sea"]').is_enabled()
     page.locator('#ec-tr-route input[value="land"]').check();page.locator('#ec-tr-items input[data-good]').first.fill('1')
     page.locator('#ec-tr-route').scroll_into_view_if_needed();shot(page,f'{cycle}-both-land-{width}')
     page.locator('#ec-tr-send').click();page.wait_for_function("document.getElementById('ec-tr-result').textContent.includes('sent')");page.wait_for_function("!document.getElementById('ec-tr-send').disabled")
     page.locator('#ec-tr-route input[value="sea"]').check();page.locator('#ec-tr-send').click();page.wait_for_function("document.getElementById('ec-tr-result').textContent.includes('aboard')")
     shot(page,f'{cycle}-sent-sea-{width}')
     page.locator('#ec-tr-to').select_option(cities[1]['id']);page.wait_for_function("document.querySelector('#ec-tr-route input[value=land]')?.disabled")
     page.locator('#ec-tr-route').scroll_into_view_if_needed();shot(page,f'{cycle}-sea-only-{width}')
     page.locator('#ec-tr-to').select_option(foreign['id']);page.wait_for_function("document.querySelector('#ec-tr-route input[value=sea]')&&!document.querySelector('#ec-tr-route input[value=sea]').disabled");page.locator('#ec-tr-route input[value="sea"]').check();page.locator('#ec-tr-send').click();page.wait_for_function("document.getElementById('ec-tr-result').textContent.startsWith('Gift:')")
     page.locator('#ec-tr-to').select_option(cities[2]['id']);page.wait_for_function("document.querySelector('#ec-tr-route input[value=sea]')?.disabled")
     shot(page,f'{cycle}-land-only-{width}')
     page.locator('#economy-body [data-tab="automation"]').click();page.wait_for_selector('#ec-so-route input[value="land"]')
     page.locator('#ec-so-to').select_option(cities[0]['id']);page.wait_for_function("document.querySelector('#ec-so-route input[value=land]')&&!document.querySelector('#ec-so-route input[value=land]').disabled")
     page.locator('#ec-so-route input[value="land"]').check();page.locator('#ec-so-out input[type=number]').first.fill('20')
     page.locator('#ec-so-route').scroll_into_view_if_needed();shot(page,f'{cycle}-standing-{width}')
     page.get_by_role('button',name='Create route',exact=True).click();page.wait_for_function("document.getElementById('ec-so-result').textContent==='Standing order created.'")
     page.locator('#drawer-economy .drawer-close').click();page.locator('#trig-diplomacy').click()
     page.locator('[data-write-list]').click();page.locator('[data-write="'+foreign['id']+'"]').click()
     page.get_by_text('Attach trade offer',exact=True).click();page.wait_for_selector('#letter-compose-route input[value="sea"]')
     page.locator('#letter-compose-text').fill('Sea trade agreement')
     page.locator('#letter-compose-good').select_option('grain');page.locator('#letter-compose-qty').fill('1');page.locator('#letter-compose-silver').fill('1');page.locator('#letter-compose-text').click()
     page.wait_for_selector('#letter-compose-route input[value="sea"]');page.locator('#letter-compose-route input[value="sea"]').check()
     page.locator('#letter-compose-route').scroll_into_view_if_needed();shot(page,f'{cycle}-trade-{width}')
     if cycle==2 and width==390:
      page.get_by_role('button',name='Send a runner',exact=True).click();page.wait_for_function("document.getElementById('letter-compose-status').textContent.includes('Dispatched')")
     assert not errors,errors
     ctx.close()
   browser.close()
  # CLI uses the same option protocol and sends explicit choices.
  for by in ['land','sea']:
   text=run(str(tmp/'keryx'),'transfer','--by',by,'--dest',cities[0]['id'],'--good','stone','--qty','1',env=clienv)
   assert 'game day' in text and 'sea' in text and 'land' in text,text
   (OUT/('keryx-'+by+'.txt')).write_text(text)
  modes=json.loads(sql(f"SELECT json_agg(t) FROM(SELECT route_mode,category,count(*) FROM transports WHERE world_id='{world}' GROUP BY route_mode,category)t"))
  assert any(t['route_mode']=='land' and t['category']=='land' for t in modes) and any(t['route_mode']=='sea' and t['category']=='naval' for t in modes),modes
  offers=json.loads(sql(f"SELECT json_agg(trade_offer) FROM messengers WHERE world_id='{world}' AND sender_id='{pid}' AND trade_offer IS NOT NULL"));assert any(o.get('by')=='sea' for o in offers),offers
  orders=api(wp+'/standing-orders',token=token);assert len(orders)==4 and all(o['by']=='land' for o in orders),orders
  (OUT/'persisted.json').write_text(json.dumps({'transports':modes,'orders':orders,'offers':offers},indent=2))
  # Additive migration is exercised only on this owned, stopped acceptance world.
  proc.terminate();proc.wait(timeout=15);proc=None
  def ground_hash():
   return {table:sql("SELECT count(*)||':'||md5(COALESCE(string_agg(j::text,'' ORDER BY j::text),'')) FROM(SELECT to_jsonb(t)-'route_mode' j FROM "+table+" t)x") for table in ['worlds','players','settlements','units','settlement_goods','transports','standing_orders','trade_routes','messengers']}
  ground=ground_hash()
  run('migrate','-path',str(ROOT/'server/db/migrations'),'-database',env['DATABASE_URL'],'down','1')
  assert sql("SELECT version||':'||dirty FROM schema_migrations")=='217:false'
  assert ground==ground_hash(),'migration down changed existing ground'
  run('migrate','-path',str(ROOT/'server/db/migrations'),'-database',env['DATABASE_URL'],'up')
  assert sql("SELECT version||':'||dirty FROM schema_migrations")=='218:false'
  assert ground==ground_hash(),'migration up changed existing ground'
  (OUT/'migration-roundtrip.json').write_text(json.dumps({'path':'218→217→218','preserved_ground_hashes':ground,'explicit_choice_metadata_removed_on_down':True},indent=2))
  assert before==hashes(),'production sources changed during capture'
  (OUT/'proof.json').write_text(json.dumps({'health':health,'shots':shots,'source_unchanged':True,'fixture':'Fresh owned PG/Redis/server; explicit island, force roster/cities/stocks/contact. Real register/found, native two1280/390 rounds choosing land then sea, unavailable alternatives and standing routes; CLI both modes, persisted transport and standing route choice.'},indent=2))
  print('PASS land/sea native transfer, unavailable alternatives, standing order and CLI persistence',flush=True)
finally:
 if proc is not None:proc.terminate();proc.wait(timeout=15)
 for name in containers:subprocess.run(['docker','rm','-fv',name],capture_output=True)
