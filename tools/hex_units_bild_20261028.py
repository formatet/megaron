#!/usr/bin/env python3
"""Hex force-card proof: fresh owned PG/Redis/server and native Firefox.
Usage: python3 tools/river_flow_bild_20261028.py OUTPUT
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
 name='megaron-hex-units-'+secrets.token_hex(4)+'-'+kind;internal='5432' if kind=='pg' else '6379'
 run('docker','run','-d','--rm','--name',name,'-p','127.0.0.1::'+internal,*args,image);containers.append(name)
 return json.loads(run('docker','inspect',name))[0]['NetworkSettings']['Ports'][internal+'/tcp'][0]['HostPort']
try:
 with tempfile.TemporaryDirectory(prefix='wiki-proof-') as td:
  tmp=Path(td);commit=run('git','-C',str(ROOT),'rev-parse','--short=8','HEAD')+'+hex-units'
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
  assert health['commit']==commit and health['migration']==221,health
  world=api('/api/v1/worlds')[0]['id'];wp='/api/v1/worlds/'+world
  token=api('/api/v1/auth/register','POST',{'username':'Dispatch-'+secrets.token_hex(4),'password':secrets.token_urlsafe(30)})['access_token']
  api(wp+'/join','POST',{},token);pid=api('/api/v1/auth/me',token=token)['id']
  def sql(q,fixture=False):
   if fixture:fixtures.append(q);(OUT/'fixture.sql').write_text('\n\n'.join(fixtures)+'\n')
   return run('docker','exec',containers[0],'psql','-U','postgres','-d','proof','-At','-v','ON_ERROR_STOP=1','-c',q)
  initial_metadata=json.loads(sql(f"SELECT json_build_object('rivers',count(*),'missing',count(*) FILTER(WHERE river_edges IS NULL)) FROM map_tiles WHERE world_id='{world}' AND terrain IN('river','river_ford')"));assert initial_metadata['rivers']>0 and initial_metadata['missing']==0,initial_metadata
  host=next(u for u in api(wp+'/units',token=token)['units'] if u['type']=='nomadic_host')
  site=json.loads(sql(f"SELECT row_to_json(t) FROM(SELECT q,r FROM map_tiles mt WHERE world_id='{world}' AND q BETWEEN 5 AND 30 AND r BETWEEN 7 AND 30 AND NOT EXISTS(SELECT 1 FROM provinces p WHERE p.world_id=mt.world_id AND ((abs(p.map_q-mt.q)<=3 AND abs(p.map_r-mt.r)<=3) OR (p.map_q IN(mt.q+5,mt.q+13) AND p.map_r=mt.r))) ORDER BY q,r LIMIT 1)t"));hq,hr=site['q'],site['r']
  # Explicit island/port fixture on unoccupied ground; orders below use normal APIs.
  sql(f"UPDATE map_tiles SET terrain='coastal_sea',coastal=false,river_edges=NULL,river_upstream_edges=NULL,river_downstream_edges=NULL,river_flow_known=NULL WHERE world_id='{world}'; UPDATE map_tiles SET terrain='plains' WHERE world_id='{world}' AND q BETWEEN {hq} AND {hq+6} AND r BETWEEN {hr} AND {hr+3}; UPDATE map_tiles SET terrain='plains',coastal=true WHERE world_id='{world}' AND q={hq+13} AND r={hr}; UPDATE map_tiles SET coastal=true WHERE world_id='{world}' AND q={hq} AND r={hr}; UPDATE units SET q={hq},r={hr},status='positioned' WHERE id='{host['id']}'; DELETE FROM sea_storms WHERE world_id='{world}';",True)
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
  config.write_text(json.dumps({'server':base,'token':token,'world_id':world,'province_id':hp}))
  from datetime import datetime,timezone,timedelta
  sql(f"UPDATE map_tiles SET terrain='coastal_sea',coastal=false,river_edges=NULL,river_upstream_edges=NULL,river_downstream_edges=NULL,river_flow_known=NULL WHERE world_id='{world}'; UPDATE map_tiles SET terrain='plains' WHERE world_id='{world}' AND q BETWEEN {hq} AND {hq+12} AND r BETWEEN {hr} AND {hr+4}; UPDATE units SET status='disbanded' WHERE world_id='{world}' AND owner_id='{pid}'; UPDATE settlements SET population=4000 WHERE id='{home}'; UPDATE settlement_goods SET amount=10000,rate=0,cap=1000000,calc_tick=current_world_tick() WHERE settlement_id='{home}';",True)
  foreign_token=api('/api/v1/auth/register','POST',{'username':'Foreign-'+secrets.token_hex(4),'password':secrets.token_urlsafe(30)})['access_token'];foreign=api('/api/v1/auth/me',token=foreign_token)['id']
  def insert(statement):return sql(statement+' RETURNING id',True).splitlines()[0]
  ship=insert(f"INSERT INTO units(world_id,owner_id,type,category,size,crew,hull,name,status,q,r,support_settlement_id,provisions,provision_goods) VALUES('{world}','{pid}','merchantman','naval',1,10,5,'White Dolphin','positioned',{hq+4},{hr-1},'{home}',100,'{{\"fish\":100}}')")
  passenger=insert(f"INSERT INTO units(world_id,owner_id,type,category,size,status,carried_by_ship_id,home_settlement_id,support_settlement_id) VALUES('{world}','{pid}','spearman','land',100,'embarked','{ship}','{home}','{home}')")
  enemies=[]
  for name in ['Red Sail']:
   enemies.append(insert(f"INSERT INTO units(world_id,owner_id,type,category,size,crew,hull,name,status,q,r) VALUES('{world}','{foreign}','galley','naval',1,30,3,'{name}','positioned',{hq+6},{hr-1})"))
  observer=insert(f"INSERT INTO units(world_id,owner_id,type,category,size,status,q,r,home_settlement_id,support_settlement_id) VALUES('{world}','{pid}','spearman','land',100,'positioned',{hq+5},{hr},'{home}','{home}')")
  for name in ['Blue Dolphin','Bronze Sail']:
   insert(f"INSERT INTO units(world_id,owner_id,type,category,size,crew,hull,name,status,q,r,support_settlement_id) VALUES('{world}','{pid}','galley','naval',1,30,4,'{name}','positioned',{hq+8},{hr-1},'{home}')")
  now=datetime.now(timezone.utc);start=int(sql(f"SELECT current_tick FROM worlds WHERE id='{world}'"));depart=now-timedelta(hours=2);arrive=now+timedelta(hours=2);path=[[hq+i,hr+2] for i in range(2,7)];route={'start_tick':start-2,'end_tick':start+2,'hexes':path,'costs':[1000]*4}
  march=insert(f"INSERT INTO units(world_id,owner_id,type,category,size,ordinal,status,q,r,target_q,target_r,march_route,depart_tick,arrive_tick,departs_at,arrives_at,home_settlement_id,support_settlement_id) VALUES('{world}','{pid}','spearman','land',100,3,'marching',{hq+2},{hr+2},{hq+6},{hr+2},'{json.dumps(route)}',{start-2},{start+2},'{depart.isoformat()}','{arrive.isoformat()}','{home}','{home}')")
  sql(f"INSERT INTO player_scouted_tiles(world_id,player_id,q,r) SELECT world_id,'{pid}',q,r FROM map_tiles WHERE world_id='{world}' AND q BETWEEN {hq} AND {hq+12} AND r BETWEEN {hr-2} AND {hr+4} ON CONFLICT DO NOTHING",True)
  roster=api(wp+'/units',token=token)['units'];walking=next(u for u in roster if u['id']==march);mq,mr=walking['current_q'],walking['current_r'];assert (mq,mr)!=(hq+2,hr+2),walking
  scenes=[('own-ship',hq+4,hr-1),('march',mq,mr),('foreign',hq+6,hr-1),('three',hq+8,hr-1)]
  # The three-force scene shares the same water hex: move a third own ship there.
  insert(f"INSERT INTO units(world_id,owner_id,type,category,size,crew,hull,name,status,q,r) VALUES('{world}','{pid}','merchantman','naval',1,10,5,'Third Sail','positioned',{hq+8},{hr-1})")
  outputs=[];observations=[]
  with sync_playwright() as pw:
   browser=pw.firefox.launch()
   for cycle in [1,2]:
    for code,q,r in scenes:
     outputs.append({'scene':code,'output':run(str(tmp/'keryx'),'hex',f'{q},{r}',env=clienv)})
     for width,height in [(1280,900),(390,844)]:
      ctx,page,errors=browser_page(browser,width,height)
      page.evaluate("async ([q,r])=>{(await import('/static/js/megaron/render/map.js')).focusDispatchHex(q,r)}",[q,r]);page.wait_for_selector('#ip-foot .hex-force')
      if width==390:
       assert page.locator('#ip-foot .hex-force[open]').count()==0
       page.screenshot(path=str(OUT/f'{cycle}-{code}-{width}-collapsed.png'))
       page.locator('#ip-foot .hex-force > summary').first.click()
      text=page.locator('#ip-foot').inner_text()
      if code=='own-ship':assert 'hull 5/5' in text and 'Passengers' in text and 'fish' in text,text
      if code=='march':assert 'Redirect' in text and 'Recall' in text and 'messenger' in text,text
      if code=='foreign':assert 'Hull: unknown' in text and 'Red Sail' in text and 'hull 3' not in text,text
      if code=='three':assert page.locator('#ip-foot .hex-force').count()==3
      assert page.locator('#inspect-panel').evaluate('(e)=>e.scrollWidth<=e.clientWidth+1')
      page.screenshot(path=str(OUT/f'{cycle}-{code}-{width}.png'));shots.append({'file':f'{cycle}-{code}-{width}.png'})
      assert not errors,errors
      if cycle==2 and code=='march' and width==1280:
       page.locator('[data-hex-redirect]').click();page.wait_for_selector('#march-ctx:visible')
       selected=page.evaluate("async()=> (await import('/static/js/megaron/state.js')).State.marchCtxUnitID");assert selected==march,selected
       page.wait_for_function("document.querySelector('#mctx-city')?.options.length>1");page.select_option('#mctx-city',home)
       with page.expect_response(lambda resp:'/units/'+march+'/recall' in resp.url and resp.request.method=='POST') as sent:
        page.locator('#mctx-send').click()
       assert sent.value.ok,sent.value.text()
       after=next(u for u in api(wp+'/units',token=token)['units'] if u['id']==march);assert after.get('orders_en_route'),after;assert after['target_q']==hq+6,after
       page.evaluate("async ([q,r])=>{(await import('/static/js/megaron/render/map.js')).focusDispatchHex(q,r)}",[mq,mr]);page.locator('[data-hex-recall]').click();page.wait_for_function("document.querySelector('[id^=hex-uorder-]')?.textContent.includes('Recall')")
       after=next(u for u in api(wp+'/units',token=token)['units'] if u['id']==march);assert after.get('orders_en_route'),after;assert after['target_q']==hq+6,after
       observations.append({'redirect_selected':selected,'orders_after_recall':after['orders_en_route'],'original_target_unchanged':True})
      ctx.close()
   browser.close()
  assert before==hashes(),'production sources changed during capture'
  (OUT/'cli.json').write_text(json.dumps(outputs,indent=2));(OUT/'proof.json').write_text(json.dumps({'health':health,'shots':shots,'source_unchanged':True,'scenes':scenes,'actions':observations,'fixture':'Owned fresh world; actual API, native Firefox twice1280/390; no replaced DOM/responses.'},indent=2));print('PASS hex forces native and actual command dispatch',flush=True)
finally:
 if proc is not None:proc.terminate();proc.wait(timeout=15)
 for name in containers:subprocess.run(['docker','rm','-fv',name],capture_output=True)
