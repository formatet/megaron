#!/usr/bin/env python3
"""Offer expiry24 proof: fresh owned PG/Redis/server and native Firefox.
Usage: python3 tools/offer24_bild_20261028.py OUTPUT
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
 name='megaron-offer24-'+secrets.token_hex(4)+'-'+kind;internal='5432' if kind=='pg' else '6379'
 run('docker','run','-d','--rm','--name',name,'-p','127.0.0.1::'+internal,*args,image);containers.append(name)
 return json.loads(run('docker','inspect',name))[0]['NetworkSettings']['Ports'][internal+'/tcp'][0]['HostPort']
try:
 with tempfile.TemporaryDirectory(prefix='offer24-proof-') as td:
  tmp=Path(td);commit=run('git','-C',str(ROOT),'rev-parse','--short=8','HEAD')+'+offer24'
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
  config.write_text(json.dumps({'server':base,'token':token,'world_id':world,'province_id':hp}))
  allcities=[{'id':home,'province_id':hp,'name':'Lato'},*cities]
  for city in allcities:
   sid=city['id'];sql(f"INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) SELECT '{sid}',key,10000,0,1000000,current_world_tick() FROM goods ON CONFLICT(settlement_id,good_key) DO UPDATE SET amount=10000,rate=0,cap=1000000,calc_tick=current_world_tick();",True)
  # A separately registered neighbour, explicitly contacted in this fresh fixture.
  other_token=api('/api/v1/auth/register','POST',{'username':'Neighbour-'+secrets.token_hex(4),'password':secrets.token_urlsafe(30)})['access_token']
  other=api('/api/v1/auth/me',token=other_token)['id']
  foreign=cities[3]
  sql(f"UPDATE settlements SET owner_id='{other}' WHERE id='{foreign['id']}'; INSERT INTO player_scouted_tiles(world_id,player_id,q,r) VALUES('{world}','{pid}',{hq+5},{hr+3}) ON CONFLICT DO NOTHING;",True)
  api(wp+'/join','POST',{},other_token)
  sql(f"INSERT INTO player_scouted_tiles(world_id,player_id,q,r) VALUES('{world}','{other}',{hq},{hr}) ON CONFLICT DO NOTHING",True)
  offers=[]
  for i,terms in enumerate([['--want-good','grain','--want-qty','1','--offer-silver','2'],['--offer-good','grain','--offer-qty','1','--want-silver','2']]):
   text=run(str(tmp/'keryx'),'trade-offer','--by','land','--to',foreign['id'],*terms,'--message','Offer expiry proof '+str(i),env=clienv);(OUT/f'cli-send-{i}.txt').write_text(text)
  records=json.loads(sql(f"SELECT json_agg(t) FROM(SELECT m.id,m.arrives_at,m.expires_at,a.due_tick arrival,e.due_tick expiry FROM messengers m JOIN scheduled_events a ON a.payload->>'messenger_id'=m.id::text AND a.event_type='MessengerArrival' JOIN scheduled_events e ON e.payload->>'messenger_id'=m.id::text AND e.event_type='OfferExpiry' WHERE m.world_id='{world}' AND m.sender_id='{pid}' ORDER BY m.id)t"))
  from datetime import datetime
  for row in records:
   assert row['expiry']-row['arrival']==24,row
   assert (datetime.fromisoformat(row['expires_at'])-datetime.fromisoformat(row['arrives_at'])).total_seconds()==24*3600,row
  current=max(r['arrival'] for r in records)
  sql(f"UPDATE worlds SET current_tick={current},last_tick_at=now() WHERE id='{world}'; UPDATE scheduled_events SET process_after=now() WHERE world_id='{world}' AND event_type='MessengerArrival';",True)
  for _ in range(200):
   statuses=sql(f"SELECT string_agg(status,',') FROM messengers WHERE world_id='{world}' AND sender_id='{pid}'")
   if statuses=='delivered,delivered':break
   time.sleep(.2)
  else:raise RuntimeError('actual Arrival worker did not deliver: '+statuses)
  stays=json.loads(sql(f"SELECT json_agg(t) FROM(SELECT m.id,s.due_tick stay,e.due_tick expiry FROM messengers m JOIN scheduled_events s ON s.payload->>'messenger_id'=m.id::text AND s.event_type='MessengerStayEnd' JOIN scheduled_events e ON e.payload->>'messenger_id'=m.id::text AND e.event_type='OfferExpiry' WHERE m.world_id='{world}' AND m.sender_id='{pid}')t"));assert all(x['stay']==x['expiry'] for x in stays),stays
  text=run(str(tmp/'keryx'),'outbox',env=clienv);assert 'expires ' in text;(OUT/'cli-outbox.txt').write_text(text)
  receiver_config=tmp/'receiver.json';receiver_config.write_text(json.dumps({'server':base,'token':other_token,'world_id':world,'province_id':foreign['province_id']}));receiver_config.chmod(0o600)
  receiver_env={**clean,'POLEIA_CONFIG':str(receiver_config)}
  text=run(str(tmp/'keryx'),'inbox',env=receiver_env);assert 'Offer expiry proof' in text;(OUT/'cli-inbox.txt').write_text(text)
  token=other_token
  with sync_playwright() as pw:
   browser=pw.firefox.launch()
   for cycle in [1,2]:
    for width,height in [(1280,900),(390,844)]:
     ctx,page,errors=browser_page(browser,width,height);page.locator('#trig-diplomacy').click();person=page.locator('[data-person="'+pid+'"]');person.wait_for();assert 'before' in person.inner_text();shot(page,f'{cycle}-deadline-{width}');person.click();page.wait_for_selector('[data-letter]');assert page.locator('#dtab-letters').get_by_role('button',name='Accept',exact=True).count()==2;shot(page,f'{cycle}-offers-{width}');assert not errors,errors;ctx.close()
   browser.close()
  assert before==hashes(),'production sources changed during capture'
  (OUT/'proof.json').write_text(json.dumps({'health':health,'shots':shots,'source_unchanged':True,'offers':records,'stays':stays,'fixture':'Fresh world, explicit contacted neighbour/cities/stocks; two real CLI buy/sell orders, saved24tick/wall deadlines, tick advance fixture and actual Arrival worker, real Keryx outbox/inbox, receiver Letters deadline and Accept/Decline in two Firefox desktop/mobile rounds.'},indent=2))
  print('PASS two actual offers,24tick deadlines, real Arrival stays, CLI and native receiver deadlines',flush=True)
finally:
 if proc is not None:proc.terminate();proc.wait(timeout=15)
 for name in containers:subprocess.run(['docker','rm','-fv',name],capture_output=True)
