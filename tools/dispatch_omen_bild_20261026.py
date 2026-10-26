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
  tmp=Path(td);commit=run('git','-C',str(ROOT),'rev-parse','--short=8','HEAD')+'+dispatch-omen'
  for binary,pkg in [('temenos','./cmd/server'),('keryx','./cmd/keryx')]:run('go','build','-ldflags','-X main.buildCommit='+commit,'-o',str(tmp/binary),pkg,cwd=ROOT/'server',env=clean)
  pg=launch('pg','postgres:16-alpine',['-e','POSTGRES_PASSWORD=proof','-e','POSTGRES_DB=proof']);redis=launch('redis','redis:7-alpine',[])
  for _ in range(100):
   if subprocess.run(['docker','exec',containers[0],'pg_isready','-U','postgres','-q'],capture_output=True).returncode==0:break
   time.sleep(.25)
  with socket.socket() as s:s.bind(('127.0.0.1',0));port=s.getsockname()[1]
  base=f'http://127.0.0.1:{port}'
  env={**clean,'DATABASE_URL':f'postgres://postgres:proof@localhost:{pg}/proof?sslmode=disable','REDIS_URL':'localhost:'+redis,'JWT_SECRET':secrets.token_urlsafe(40),'PORT':str(port),'TICK_SECONDS':'6','MAP_WIDTH':'56','MAP_HEIGHT':'40','WORLD_NAME':'Dispatch and omen proof','POLEIA_WORLD_START_WANAXES':'1','STATIC_DIR':str(ROOT/'web/static'),'TEMPLATE_DIR':str(ROOT/'web/templates'),'CHRONICLE_DIR':str(tmp/'chronicles'),'REPORTS_DIR':str(tmp/'reports')}
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
  assert health['commit']==commit and health['migration']==215,health
  world=api('/api/v1/worlds')[0]['id'];wp='/api/v1/worlds/'+world
  token=api('/api/v1/auth/register','POST',{'username':'Dispatch-'+secrets.token_hex(4),'password':secrets.token_urlsafe(30)})['access_token']
  api(wp+'/join','POST',{},token);pid=api('/api/v1/auth/me',token=token)['id']
  def sql(q,fixture=False):
   if fixture:fixtures.append(q);(OUT/'fixture.sql').write_text('\n\n'.join(fixtures)+'\n')
   return run('docker','exec',containers[0],'psql','-U','postgres','-d','proof','-At','-v','ON_ERROR_STOP=1','-c',q)
  host=next(u for u in api(wp+'/units',token=token)['units'] if u['type']=='nomadic_host')
  site=json.loads(sql(f"SELECT row_to_json(t) FROM(SELECT t.q,t.r FROM map_tiles t WHERE t.world_id='{world}' AND t.q>8 AND t.r>8 AND t.q<40 AND t.r<30 AND EXISTS(SELECT 1 FROM map_tiles e WHERE e.world_id=t.world_id AND e.q=t.q+7 AND e.r=t.r) ORDER BY t.q,t.r LIMIT 1)t"));hq,hr=site['q'],site['r'];cq=hq+6;cr=hr;fx=cq+1;fr=cr
  sql(f"UPDATE map_tiles SET terrain='plains',coastal=false WHERE world_id='{world}'; UPDATE units SET q={hq},r={hr},status='positioned' WHERE id='{host['id']}';",True)
  home=api(wp+'/founding/settle','POST',{'name':'Lato'},token)['settlement_id'];hp=sql(f"SELECT province_id FROM settlements WHERE id='{home}'")
  cp=sql(f"SELECT id FROM provinces WHERE world_id='{world}' AND map_q={cq} AND map_r={cr}")
  if not cp:cp=sql(f"INSERT INTO provinces(world_id,map_q,map_r,terrain_type,coastal) VALUES('{world}',{cq},{cr},'plains',true) RETURNING id;",True).splitlines()[0]
  city=sql(f"INSERT INTO settlements(world_id,province_id,name,culture_id,owner_id,control_type,is_capital,state,population) VALUES('{world}','{cp}','Petras','minoan','{pid}','colony',false,'active',2000) RETURNING id;",True).splitlines()[0]
  sql(f"UPDATE settlements SET culture_id='minoan',population=2000 WHERE id='{home}'; UPDATE provinces SET coastal=true WHERE id='{cp}'; UPDATE map_tiles SET terrain='forest_olive_grove' WHERE world_id='{world}' AND q={fx} AND r={fr}; UPDATE map_tiles SET coastal=true WHERE world_id='{world}' AND q={cq} AND r={cr}; INSERT INTO buildings(settlement_id,building_type,level,god_id) VALUES('{city}','temple',3,'potnia'),('{city}','shipyard',1,NULL),('{city}','harbour',1,NULL) ON CONFLICT DO NOTHING; INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) SELECT '{city}',key,10000,0,1000000,current_world_tick() FROM goods ON CONFLICT(settlement_id,good_key) DO UPDATE SET amount=10000,rate=0,cap=1000000,calc_tick=current_world_tick(); UPDATE settlement_goods SET amount=10000,rate=0,cap=1000000,calc_tick=current_world_tick() WHERE settlement_id='{home}';",True)
  # Actual placement APIs maintain the temple and drive the shipyard worker.
  for body in [{'target_kind':'building','building_type':'temple','god':'potnia','good_key':'cult'},{'target_kind':'building','building_type':'shipyard','good_key':'shipbuilding'}]:
   result=api(wp+'/provinces/'+cp+'/placements','POST',body,token);(OUT/(body['building_type']+'-placement.json')).write_text(json.dumps(result,indent=2))
  sql(f"INSERT INTO player_world_records(player_id,world_id,kharis_amount,kharis_cap,kharis_calc_tick,kharis_rate) VALUES('{pid}','{world}',95,100,current_world_tick(),0) ON CONFLICT(player_id,world_id) DO UPDATE SET kharis_amount=95,kharis_cap=100,kharis_calc_tick=current_world_tick(),kharis_rate=0;",True)
  foreign=sql("INSERT INTO players(username,password_hash) VALUES('foreign-"+secrets.token_hex(4)+"','x') RETURNING id;",True).splitlines()[0]
  sql(f"INSERT INTO gossip_events(world_id,recipient_id,source_region,category,text,transport_owner_id) VALUES('{world}','{pid}','Fixture harbour','trade','OWN TRANSPORT FIXTURE must be hidden','{pid}'),('{world}','{pid}','Foreign harbour','trade','Foreign copper cargo is travelling along a distant road','{foreign}');",True)
  build=api(wp+'/provinces/'+cp+'/build','POST',{'building_type':'lumbermill','hex_q':fx,'hex_r':fr},token);(OUT/'build-order.json').write_text(json.dumps(build,indent=2))
  (OUT/'build-queue-api.json').write_text(json.dumps(api(wp+'/provinces/'+cp,token=token),indent=2))
  recruit=api(wp+'/provinces/'+cp+'/recruit','POST',{'unit_type':'merchantman','name':'Petras Proof'},token);unit=recruit['unit_id'];(OUT/'recruit-order.json').write_text(json.dumps(recruit,indent=2))
  route=api(wp+'/standing-orders','POST',{'from_settlement_id':home,'to_settlement_id':city,'crewed_by_settlement_id':home,'outbound':[{'good_key':'grain','threshold':20000}],'return':[]},token);(OUT/'route-order.json').write_text(json.dumps(route,indent=2))
  cfg=tmp/'keryx.json';cfg.write_text(json.dumps({'server':base,'token':token,'world_id':world,'province_id':cp}));cfg.chmod(0o600);clienv={**clean,'POLEIA_CONFIG':str(cfg)}
  (OUT/'build-queue-keryx.txt').write_text(run(str(tmp/'keryx'),'build','--queue',env=clienv)+'\n')
  deadline=time.monotonic()+100
  while time.monotonic()<deadline:
   notes=api(wp+'/notifications',token=token)['notifications'];gossip=api(wp+'/gossip',token=token)
   buildnote=next((n for n in notes if n['kind']=='BuildComplete' and n['body'].get('settlement_id')==city),None)
   trainnote=next((n for n in notes if n['kind']=='TrainComplete' and n['body'].get('unit_id')==unit),None)
   omens=[g for g in gossip if g.get('omen',{}).get('true') is True]
   if buildnote and trainnote and omens:break
   time.sleep(.5)
  assert buildnote and trainnote and omens,(buildnote,trainnote,omens)
  assert buildnote['body']['settlement_name']=='Petras' and buildnote['body']['hex_q']==fx and buildnote['body']['hex_r']==fr,buildnote
  assert trainnote['body']['unit_name']=='Emporos' and trainnote['body']['settlement_name']=='Petras',trainnote
  assert all('OWN TRANSPORT FIXTURE' not in g['text'] for g in gossip) and any('Foreign copper cargo' in g['text'] for g in gossip),gossip
  (OUT/'gossip-api.json').write_text(json.dumps(gossip,indent=2));(OUT/'notifications-api.json').write_text(json.dumps(notes,indent=2))
  (OUT/'maintained-db.json').write_text(sql(f"SELECT json_agg(t) FROM(SELECT event_type,payload FROM events WHERE world_id='{world}' AND event_type IN ('KharisMaintained','OmenObserved'))t"))
  (OUT/'omen-db.json').write_text(sql(f"SELECT json_agg(t) FROM(SELECT source_region,text,fact_data FROM gossip_events WHERE world_id='{world}' AND category='omen')t"))
  (OUT/'notification-keryx.txt').write_text(run(str(tmp/'keryx'),'notifications','--all',env=clienv)+'\n')
  rite=api(wp+'/settlements/'+city+'/rite','POST',{'prayer':'minoan_potnia_walls'},token);(OUT/'rite-result.json').write_text(json.dumps(rite,indent=2))
  with sync_playwright() as pw:
   browser=pw.firefox.launch();pages=[]
   for width,height in [(1280,900),(390,844)]:
    ctx=browser.new_context(viewport={'width':width,'height':height});ctx.add_cookies([{'name':'poleia_token','value':token,'url':base}]);ctx.add_init_script('localStorage.setItem("poleia_token",'+json.dumps(token)+')')
    page=ctx.new_page();errors=[];page.on('pageerror',lambda e,errors=errors:errors.append(str(e)));page.goto(base+'/world/'+world+'/map',wait_until='networkidle')
    if page.locator('#dc-dismiss-all').is_visible():page.locator('#dc-dismiss-all').click()
    for brief in page.locator('.lb-dismiss').all():
     if brief.is_visible():brief.click()
    pages.append((page,width,errors))
   def shot(page,width,code):
    page.wait_for_timeout(400);page.screenshot(path=str(OUT/f'{code}-{width}.png'));shots.append({'phase':code,'width':width,'file':f'{code}-{width}.png'})
   def visible_drawer(page,name):
    page.wait_for_function('(name)=>{const a=[...document.querySelectorAll(".drawer.open")];return a.length===1&&a[0].id==="drawer-"+name}',arg=name)
   def feed(page):
    page.evaluate("async()=>{if(!document.querySelector('#drawer-notif.open'))window.toggleDrawer('notif');await (await import('/static/js/megaron/ui/drawers/notif.js')).loadNotifDrawer()}")
    page.wait_for_timeout(200)
   for page,width,errors in pages:
    feed(page);text=page.locator('#notif-body').inner_text();assert 'Omen' in text and 'Foreign copper cargo' in text and 'OWN TRANSPORT FIXTURE' not in text,text
    page.locator('#notif-body .notif-list-item').filter(has_text='Omen').first.scroll_into_view_if_needed();shot(page,width,'omen-feed');(OUT/f'feed-{width}.txt').write_text(text)
    page.locator('#notif-body .notif-list-item').filter(has_text='Lumbermill built').first.click();page.wait_for_selector('#dw-action-btn');assert page.locator('#dw-action-btn').inner_text()=='Staff Lumbermill';shot(page,width,'build-dispatch')
    page.locator('#dw-action-btn').click();page.wait_for_selector('#city-gubbe-grid .gubbe-hex.selected')
    page.wait_for_function('(id)=>window.document.querySelector("#city-drawer-title")?.textContent.includes("Petras")',arg=cp)
    detail=page.locator('#gubbe-detail').inner_text();assert f'({fx},{fr})' in detail,detail
    selected=page.locator('#city-gubbe-grid .gubbe-hex.selected').locator('..').get_attribute('data-ordinal')
    opts=api(wp+'/provinces/'+cp+'/placement-options',token=token);expect=next(h['hex_ordinal'] for h in opts['hexes'] if h['hex_q']==fx and h['hex_r']==fr);assert int(selected)==expect,(selected,expect)
    (OUT/f'staff-selected-{width}.json').write_text(json.dumps({'province_id':cp,'site':[fx,fr],'ordinal':expect,'detail':detail},indent=2));page.locator('#gubbe-detail').scroll_into_view_if_needed();visible_drawer(page,'city');shot(page,width,'staff-selected')
    feed(page);page.locator('#notif-body .notif-list-item').filter(has_text='Emporos ready in Petras').first.click();page.wait_for_selector('#dw-action-btn');assert page.locator('#dw-action-btn').inner_text()=='Show the ship';shot(page,width,'train-dispatch')
    page.locator('#dw-action-btn').click();page.wait_for_selector('#ucard-'+unit+'.ucard-focus');page.locator('#ucard-'+unit).scroll_into_view_if_needed();visible_drawer(page,'war');shot(page,width,'unit-focused')
    feed(page);page.locator('#notif-body .notif-list-item').filter(has_text='Mistress of the Walls').first.click();page.wait_for_selector('#dw-action-btn');assert 'Petras' in page.locator('#dw-action-btn').inner_text();page.locator('#dw-action-btn').click();page.wait_for_function("document.querySelector('#kult-body')?.textContent.includes('Cult in Petras')");visible_drawer(page,'kult');shot(page,width,'cult-selected')
    # Route action, when the worker emitted one, validates the real saved route before navigation.
    feed(page);routeRows=page.locator('#notif-body .notif-list-item').filter(has_text='Standing delivery setting out')
    if routeRows.count():
     routeRows.first.click()
     if page.locator('#dw-action-btn').count():
      label=page.locator('#dw-action-btn').inner_text()
      if label.startswith('Manage route'):
       page.locator('#dw-action-btn').click();page.wait_for_selector('[id^="standing-order-"].ucard-focus');visible_drawer(page,'economy');shot(page,width,'route-selected')
    assert not errors,errors
   browser.close()
  assert before==hashes(),'production sources changed during capture'
  (OUT/'proof.json').write_text(json.dumps({'health':health,'source_sha256':before,'source_unchanged':True,'fixture':'Fresh owned register/join/found Lato, explicit second own city Petras/forest/temple3/shipyard/harbour/stock fixtures. Actual temple+shipyard placements, actual build/recruit/route/rite APIs and worker completion. Real maintained-temple omen; own/foreign transport rumor rows explicitly fixture-marked. All browser content/dispatch rows are real server API data; no response or DOM replacement.','omen_count':len(omens),'browser_errors':[],'shots':shots},indent=2));print('PASS real BuildComplete staff exactcolonyhex +Emporosunitfocus +templemaintainedOmenfeed +owntransportgossipfilter, native1280/390, stable sources',flush=True)
finally:
 if proc is not None:proc.terminate();proc.wait(timeout=15)
 for name in containers:subprocess.run(['docker','rm','-fv',name],capture_output=True)
