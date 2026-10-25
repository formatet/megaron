#!/usr/bin/env python3
"""Owned fresh runtime: empty/laden ship targeting and real three-passenger city landing.
Usage: python3 tools/city_landing_bild_20261025.py OUTPUT
Native Firefox, real register/join/found/load/march. Explicit corridor/city/unit fixtures logged.
"""
from pathlib import Path
import datetime,os,json,secrets,socket,subprocess,tempfile,time,urllib.request,urllib.error,hashlib,sys
from playwright.sync_api import sync_playwright
ROOT=Path(__file__).resolve().parents[1];OUT=Path(sys.argv[1]).resolve();OUT.mkdir(parents=True,exist_ok=True)
containers=[];proc=None;clean={k:os.environ[k] for k in ('HOME','PATH')};clean['CGO_ENABLED']='0'
def run(*args,**kw):return subprocess.check_output(args,text=True,**kw).strip()
def hashes():return {str(p.relative_to(ROOT)):hashlib.sha256(p.read_bytes()).hexdigest() for d in ('server','web') for p in sorted((ROOT/d).rglob('*')) if p.is_file() and p.suffix in ('.go','.js','.md','.html','.sql','.css','.json') and not p.name.endswith('_test.go')}
def launch(kind,image,args):
 name='megaron-city-land-'+secrets.token_hex(4)+'-'+kind;internal='5432' if kind=='pg' else '6379'
 run('docker','run','-d','--rm','--name',name,'-p','127.0.0.1::'+internal,*args,image);containers.append(name)
 return json.loads(run('docker','inspect',name))[0]['NetworkSettings']['Ports'][internal+'/tcp'][0]['HostPort']
fixtures=[];shots=[];before=hashes();(OUT/'source-before.json').write_text(json.dumps(before,indent=2))
try:
 with tempfile.TemporaryDirectory(prefix='city-landing-proof-') as td:
  tmp=Path(td);commit=run('git','-C',str(ROOT),'rev-parse','--short=8','HEAD')+'+city-landing'
  for binary,pkg in [('temenos','./cmd/server'),('keryx','./cmd/keryx')]:run('go','build','-ldflags','-X main.buildCommit='+commit,'-o',str(tmp/binary),pkg,cwd=ROOT/'server',env=clean)
  pg=launch('pg','postgres:16-alpine',['-e','POSTGRES_PASSWORD=landing','-e','POSTGRES_DB=landing']);redis=launch('redis','redis:7-alpine',[])
  for _ in range(100):
   if subprocess.run(['docker','exec',containers[0],'pg_isready','-U','postgres','-q'],capture_output=True).returncode==0:break
   time.sleep(.25)
  with socket.socket() as s:s.bind(('127.0.0.1',0));port=s.getsockname()[1]
  base=f'http://127.0.0.1:{port}'
  env={**clean,'DATABASE_URL':f'postgres://postgres:landing@localhost:{pg}/landing?sslmode=disable','REDIS_URL':'localhost:'+redis,'JWT_SECRET':secrets.token_urlsafe(40),'PORT':str(port),'TICK_SECONDS':'6','MAP_WIDTH':'56','MAP_HEIGHT':'40','WORLD_NAME':'Petras landing proof','POLEIA_WORLD_START_WANAXES':'1','STATIC_DIR':str(ROOT/'web/static'),'TEMPLATE_DIR':str(ROOT/'web/templates'),'CHRONICLE_DIR':str(tmp/'chronicles'),'REPORTS_DIR':str(tmp/'reports')}
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
  assert health['commit']==commit,health
  world=api('/api/v1/worlds')[0]['id'];wp='/api/v1/worlds/'+world
  token=api('/api/v1/auth/register','POST',{'username':'CityLanding-'+secrets.token_hex(4),'password':secrets.token_urlsafe(30)})['access_token']
  api(wp+'/join','POST',{},token);pid=api('/api/v1/auth/me',token=token)['id']
  def sql(q,fixture=False):
   if fixture:fixtures.append(q);(OUT/'fixture.sql').write_text('\n\n'.join(fixtures)+'\n')
   return run('docker','exec',containers[0],'psql','-U','postgres','-d','landing','-At','-v','ON_ERROR_STOP=1','-c',q)
  host=next(u for u in api(wp+'/units',token=token)['units'] if u['type']=='nomadic_host')
  site=json.loads(sql(f"SELECT row_to_json(t) FROM(SELECT t.q,t.r FROM map_tiles t WHERE t.world_id='{world}' AND t.q>8 AND t.r>8 AND t.q<40 AND t.r<30 AND EXISTS(SELECT 1 FROM map_tiles e WHERE e.world_id=t.world_id AND e.q=t.q+6 AND e.r=t.r) ORDER BY t.q,t.r LIMIT 1)t"));hq,hr=site['q'],site['r']
  sql(f"UPDATE map_tiles SET terrain='plains',coastal=false WHERE world_id='{world}'; UPDATE units SET q={hq},r={hr},status='positioned' WHERE id='{host['id']}';",True)
  home=api(wp+'/founding/settle','POST',{'name':'Lato'},token)['settlement_id']
  hp=sql(f"SELECT province_id FROM settlements WHERE id='{home}'")
  dp=sql(f"SELECT id FROM provinces WHERE world_id='{world}' AND map_q={hq+6} AND map_r={hr}")
  if not dp:dp=sql(f"INSERT INTO provinces(world_id,map_q,map_r,terrain_type,coastal) VALUES('{world}',{hq+6},{hr},'plains',true) RETURNING id;",True).splitlines()[0]
  dest=sql(f"INSERT INTO settlements(world_id,province_id,name,culture_id,owner_id,control_type,is_capital,state,population) VALUES('{world}','{dp}','Petras','achaean','{pid}','colony',false,'active',5000) RETURNING id;",True).splitlines()[0]
  sql(f"UPDATE provinces SET coastal=true WHERE id IN ('{hp}','{dp}'); UPDATE map_tiles SET terrain='coastal_sea' WHERE world_id='{world}' AND r={hr} AND q BETWEEN {hq+1} AND {hq+5}; UPDATE map_tiles SET coastal=true WHERE world_id='{world}' AND r={hr} AND q IN ({hq},{hq+6}); DELETE FROM sea_storms WHERE world_id='{world}'; UPDATE settlements SET population=5000 WHERE id='{home}'; UPDATE settlement_goods SET amount=100000,rate=0,cap=1000000,calc_tick=current_world_tick() WHERE settlement_id='{home}'; INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) SELECT '{dest}',key,100000,0,1000000,current_world_tick() FROM goods ON CONFLICT DO NOTHING;",True)
  ship=sql(f"INSERT INTO units(world_id,owner_id,type,category,size,crew,hull,status,q,r,settlement_id,support_settlement_id,name) VALUES('{world}','{pid}','merchantman','naval',1,10,5,'garrison',{hq},{hr},'{home}','{home}','Petras Carrier') RETURNING id;",True).splitlines()[0]
  cargo=[]
  for i in range(3):cargo.append(sql(f"INSERT INTO units(world_id,owner_id,type,category,size,status,q,r,settlement_id,support_settlement_id,ordinal) VALUES('{world}','{pid}','spearman','land',100,'garrison',{hq},{hr},'{home}','{home}',{i+3}) RETURNING id;",True).splitlines()[0])
  cfg=tmp/'keryx.json';cfg.write_text(json.dumps({'server':base,'token':token,'world_id':world,'province_id':hp}));cfg.chmod(0o600);clienv={**clean,'POLEIA_CONFIG':str(cfg)}
  cities=api(wp+'/cities',token=token);(OUT/'cities.json').write_text(json.dumps(cities,indent=2));city=next(c for c in cities if c['settlement_id']==dest);assert city['own'] and (city.get('coastal') or city.get('has_harbour')),city
  with sync_playwright() as pw:
   browser=pw.firefox.launch();pages=[]
   for width,height in [(1280,900),(390,844)]:
    ctx=browser.new_context(viewport={'width':width,'height':height});ctx.add_cookies([{'name':'poleia_token','value':token,'url':base}]);ctx.add_init_script('localStorage.setItem("poleia_token",'+json.dumps(token)+')')
    page=ctx.new_page();errors=[];page.on('pageerror',lambda e,errors=errors:errors.append(str(e)))
    page.goto(base+'/world/'+world+'/map',wait_until='networkidle')
    if page.locator('#dc-dismiss-all').is_visible():page.locator('#dc-dismiss-all').click()
    for brief in page.locator('.lb-dismiss').all():
     if brief.is_visible():brief.click()
    pages.append((page,width,errors))
   def target(page):
    page.locator('#trig-war').click();page.locator('#war-body button[onclick^="unitMarch("][onclick*="'+ship+'"]').click()
    page.wait_for_function("!document.querySelector('#mctx-city')?.disabled")
    page.locator('#mctx-city').select_option(dest)
    page.wait_for_selector('#mg-0');page.wait_for_function("!document.querySelector('#mctx-eta')?.textContent.includes('Estimating arrival')")
    assert page.locator('#mg-0').input_value()=='1'
    assert 'Petras Carrier' in page.locator('#mctx-units').inner_text()
   def shot(page,width,code):
    page.screenshot(path=str(OUT/f'{code}-{width}.png'));shots.append({'phase':code,'width':width,'file':f'{code}-{width}.png'})
   for page,width,errors in pages:
    target(page);(OUT/f'empty-{width}.txt').write_text(page.locator('#march-ctx').inner_text());shot(page,width,'empty');page.locator('#march-ctx .mctx-close').click()
   loads=[]
   for unit in cargo:loads.append(api(wp+'/units/'+ship+'/load','POST',{'unit_id':unit},token))
   (OUT/'loads.json').write_text(json.dumps(loads,indent=2));(OUT/'loaded-api.json').write_text(json.dumps(api(wp+'/units',token=token),indent=2))
   for page,width,errors in pages:
    target(page);text=page.locator('#march-ctx').inner_text();assert 'ship docks and stays' in text,text
    (OUT/f'loaded-{width}.txt').write_text(text);shot(page,width,'loaded')
   page=pages[0][0]
   with page.expect_response(lambda r:r.request.method=='POST' and r.url.endswith('/march')) as submitted:page.locator('#mctx-send').click()
   response=submitted.value;assert response.status in (200,201,202),response.text();order=response.json();(OUT/'order.json').write_text(json.dumps(order,indent=2))
   active=json.loads(sql(f"SELECT row_to_json(t) FROM(SELECT status,arrives_at,march_intent,target_q,target_r,land_target_q,land_target_r,support_settlement_id FROM units WHERE id='{ship}')t"));assert active['march_intent']=='dock',active
   (OUT/'active-db.json').write_text(json.dumps(active,indent=2));deadline=datetime.datetime.fromisoformat(active['arrives_at'])
   (OUT/'sailing-keryx.txt').write_text(run(str(tmp/'keryx'),'unit','list',env=clienv)+'\n')
   for page,width,errors in pages:
    if width==390:page.locator('#march-ctx .mctx-close').click()
    shot(page,width,'ordered')
   timeout=time.monotonic()+120
   while time.monotonic()<timeout:
    if sql(f"SELECT status FROM units WHERE id='{ship}'")=='garrison':break
    time.sleep(.5)
   rows=json.loads(sql(f"SELECT json_agg(t) FROM(SELECT id,status,q,r,settlement_id,support_settlement_id,carried_by_ship_id,march_intent FROM units WHERE id IN ('{ship}',"+','.join("'"+i+"'" for i in cargo)+"))t"))
   assert len(rows)==4 and all(r['status']=='garrison' and r['settlement_id']==dest and r['support_settlement_id']==home and r['carried_by_ship_id'] is None and r['q']==hq+6 and r['r']==hr for r in rows),rows
   (OUT/'arrival-db.json').write_text(json.dumps(rows,indent=2));(OUT/'arrival-api.json').write_text(json.dumps(api(wp+'/units',token=token),indent=2))
   (OUT/'arrival-keryx.txt').write_text(run(str(tmp/'keryx'),'unit','list',env=clienv)+'\n')
   for page,width,errors in pages:
    page.reload(wait_until='networkidle');page.locator('#trig-war').click();page.wait_for_function("document.querySelector('#war-body')?.textContent.includes('5th Spearmen of Lato')")
    text=page.locator('#war-body').inner_text();assert all(name in text for name in ['3rd Spearmen of Lato','4th Spearmen of Lato','5th Spearmen of Lato','Petras Carrier']),text
    for brief in page.locator('#drawer-war .lb-dismiss').all():
     if brief.is_visible():brief.click()
    page.wait_for_timeout(400)
    page.locator('#ucard-'+ship).evaluate("el=>el.scrollIntoView({block:'start'})")
    page.wait_for_timeout(100)
    for unit in [ship,*cargo]:
     rect=page.locator('#ucard-'+unit).bounding_box();assert rect and rect['x']>=0 and rect['x']+rect['width']<=width+1 and rect['y']>=0 and rect['y']+rect['height']<=page.viewport_size['height'],(unit,width,rect)
    (OUT/f'arrival-{width}.txt').write_text(text);shot(page,width,'arrival');assert not errors,errors
   browser.close()
  pending=int(sql(f"SELECT count(*) FROM scheduled_events WHERE world_id='{world}' AND payload->>'unit_id'='{ship}' AND processed_at IS NULL"));assert pending==0,pending
  assert before==hashes(),'production sources changed during proof'
  (OUT/'proof.json').write_text(json.dumps({'health':health,'source_sha256':before,'source_unchanged':True,'fixture':'Fresh owned rig. Real register/join/found Lato, explicit Petras/corridor/merchantman/three ready cohorts, real three load APIs and native War→March→Destination city, actual UI march/deadline/worker arrival. All four arrive garrison in Petras, original Lato support unchanged, no pending return.','deadline':deadline.isoformat(),'pending_ship_events':pending,'browser_errors':[],'shots':shots},indent=2));print('PASS native Firefox1280/390 empty/laden city target, actual3load+UIorder+arrival, same support and no return; stable sources',flush=True)
finally:
 if proc is not None:proc.terminate();proc.wait(timeout=15)
 for name in containers:subprocess.run(['docker','rm','-fv',name],capture_output=True)
