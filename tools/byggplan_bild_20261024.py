#!/usr/bin/env python3
"""BILD-only: owned fresh runtime, real worker/HTTP/WS and native Firefox.
Usage: python3 tools/byggplan_bild_20261024.py OUTPUT
Explicit scene fixtures are logged. No production source changes or shared rig.
"""
from pathlib import Path
import os,json,secrets,socket,subprocess,tempfile,time,urllib.request,urllib.error,hashlib,sys
from playwright.sync_api import sync_playwright
ROOT=Path(__file__).resolve().parents[1]
OUT=Path(sys.argv[1]).resolve();OUT.mkdir(parents=True,exist_ok=True)
containers=[];proc=None
clean={k:os.environ[k] for k in ('HOME','PATH')};clean['CGO_ENABLED']='0'
def run(*args,**kw):return subprocess.check_output(args,text=True,**kw).strip()
def hashes():return {str(p.relative_to(ROOT)):hashlib.sha256(p.read_bytes()).hexdigest() for d in ('server','web') for p in sorted((ROOT/d).rglob('*')) if p.is_file() and p.suffix in ('.go','.js','.md','.html','.sql') and not p.name.endswith('_test.go')}
before=hashes();fixtures=[];shots=[];observations=[]
def launch(kind,image,args):
 name='megaron-bildplan-'+secrets.token_hex(4)+'-'+kind;internal='5432' if kind=='pg' else '6379'
 run('docker','run','-d','--rm','--name',name,'-p','127.0.0.1::'+internal,*args,image);containers.append(name)
 return json.loads(run('docker','inspect',name))[0]['NetworkSettings']['Ports'][internal+'/tcp'][0]['HostPort']
SPY=r'''(()=>{
 window.__proofAudio=[];
 const ac=window.AudioContext||window.webkitAudioContext;
 if(ac){const original=ac.prototype.createOscillator;ac.prototype.createOscillator=function(...args){
  const osc=original.apply(this,args),set=osc.frequency.setValueAtTime.bind(osc.frequency);
  osc.frequency.setValueAtTime=function(value,at){window.__proofAudio.push({frequency:value,at,observed:performance.now()});return set(value,at)};return osc;
 }}
})()'''
try:
 with tempfile.TemporaryDirectory(prefix='bildplan-proof-') as td:
  tmp=Path(td);commit=run('git','-C',str(ROOT),'rev-parse','--short=8','HEAD')
  run('go','build','-ldflags','-X main.buildCommit='+commit,'-o',str(tmp/'temenos'),'./cmd/server',cwd=ROOT/'server',env=clean)
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
  assert health['commit']==commit and health['migration']==210,health
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
  me=actor('Leandros');writer=actor('Myrine');fallen=actor('Dorian')
  # A coastal site with room for a short dry corridor to the other city.
  site=json.loads(sql(f"SELECT row_to_json(p) FROM (SELECT a.q,a.r FROM map_tiles a WHERE a.world_id='{world}' AND a.terrain='plains' AND a.coastal AND EXISTS(SELECT 1 FROM map_tiles b WHERE b.world_id=a.world_id AND b.q=a.q+6 AND b.r=a.r) AND EXISTS(SELECT 1 FROM map_tiles b WHERE b.world_id=a.world_id AND b.q=a.q-1 AND b.r=a.r+2) AND EXISTS(SELECT 1 FROM map_tiles b WHERE b.world_id=a.world_id AND b.q=a.q AND b.r=a.r+1) ORDER BY a.q,a.r LIMIT 1)p"))
  q,r=site['q'],site['r'];fq,fr=q-1,r+1
  sql(f"UPDATE map_tiles SET terrain='plains',coastal=true WHERE world_id='{world}' AND r={r} AND q BETWEEN {q} AND {q+6}; UPDATE map_tiles SET terrain='coastal_sea' WHERE world_id='{world}' AND (q,r) IN(({fq},{fr}),({fq+1},{fr}),({fq},{fr+1}));",True)
  for who,x,name in [(me,q,'Lato'),(writer,q+6,'Gournia')]:
   sql(f"UPDATE units SET q={x},r={r},status='positioned' WHERE id='{who['host']}';",True)
   who['sid']=api(wp+'/founding/settle','POST',{'name':name},who['token'])['settlement_id']
   sql(f"UPDATE units SET q={x},r={r},status='garrison',settlement_id='{who['sid']}',support_settlement_id='{who['sid']}' WHERE world_id='{world}' AND owner_id='{who['pid']}' AND type='spearman';",True)
   who['province']=next(p['id'] for p in api(wp+'/provinces',token=who['token']) if p.get('settlement_id')==who['sid'])
  sql(f"UPDATE units SET q={q+2},r={r},status='positioned' WHERE id='{fallen['host']}'; UPDATE founder_phase SET is_refugee=true WHERE host_unit_id='{fallen['host']}'; UPDATE worlds SET current_tick=100,last_tick_at=now() WHERE id='{world}';",True)
  for a,b in [(me,writer),(writer,me)]:sql(f"INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('{world}','{a['pid']}','{b['province']}') ON CONFLICT DO NOTHING",True)
  sql(f"UPDATE provinces SET coastal=true WHERE id='{me['province']}'; INSERT INTO buildings(settlement_id,building_type,level,god_id) VALUES('{me['sid']}','harbour',1,NULL),('{me['sid']}','temple',3,'potnia') ON CONFLICT DO NOTHING; UPDATE player_world_records SET kharis_amount=90,kharis_rate=0,kharis_calc_tick=100 WHERE world_id='{world}' AND player_id='{me['pid']}'; UPDATE settlement_goods SET amount=10000,rate=0,cap=1000000,calc_tick=100 WHERE settlement_id IN('{me['sid']}','{writer['sid']}'); DELETE FROM settlement_placement WHERE settlement_id='{me['sid']}'; INSERT INTO settlement_placement(settlement_id,gubbe_ordinal,target_kind,hex_q,hex_r,good_key) VALUES('{me['sid']}',1,'hex',{fq},{fr},'fish');",True)
  storm=sql(f"INSERT INTO sea_storms(world_id,heading,created_tick) VALUES('{world}',0,100) RETURNING id",True).splitlines()[0]
  sql(f"INSERT INTO sea_storm_track(storm_id,tick,slot,q,r) VALUES('{storm}',100,0,{fq},{fr}),('{storm}',100,1,{fq+1},{fr}),('{storm}',100,2,{fq},{fr+1});",True)
  first=api(wp+'/settlements/'+writer['sid']+'/messengers','POST',{'destination_id':me['sid'],'message':'Leandros, the crossing is clear. Shall we meet by the harbour? — Myrine'},writer['token'])
  sql(f"UPDATE scheduled_events SET due_tick=100,process_after=now() WHERE world_id='{world}' AND event_type='MessengerArrival' AND payload->>'messenger_id'='{first['id']}';",True)
  for _ in range(150):
   if sql(f"SELECT status FROM messengers WHERE id='{first['id']}'")=='delivered':break
   time.sleep(.2)
  else:raise RuntimeError('actual Arrival worker did not deliver')
  notification=json.loads(sql(f"SELECT row_to_json(n) FROM(SELECT kind,body_json,created_at FROM notifications WHERE world_id='{world}' AND player_id='{me['pid']}' AND kind='MessengerArrival' ORDER BY created_at DESC LIMIT 1)n"))
  troop=next(u['id'] for u in api(wp+'/units',token=me['token'])['units'] if u['type']=='spearman')
  snapshot=api(wp+'/provinces/'+me['province'],token=me['token']);(OUT/'city-before.json').write_text(json.dumps(snapshot,indent=2));assert snapshot['settlement']['storm_no_fishing']==1,snapshot['settlement'].get('storm_no_fishing')
  foreign=api(wp+'/foreign-units',token=me['token']);(OUT/'foreign-units.json').write_text(json.dumps(foreign,indent=2))
  # Make a single unknown ore target with its full nineteen-hex patch in the map.
  target=json.loads(sql(f"SELECT row_to_json(p) FROM (SELECT a.q,a.r FROM map_tiles a WHERE a.world_id='{world}' AND a.terrain NOT IN('coastal_sea','deep_sea') AND abs(a.q-{q})+abs(a.r-{r})>18 AND NOT EXISTS(SELECT 1 FROM player_scouted_tiles p WHERE p.world_id=a.world_id AND p.player_id='{me['pid']}' AND p.q=a.q AND p.r=a.r) AND NOT EXISTS(SELECT 1 FROM units u WHERE u.world_id=a.world_id AND u.owner_id='{me['pid']}' AND u.status!='disbanded' AND (abs(u.q-a.q)+abs(u.q+u.r-a.q-a.r)+abs(u.r-a.r))/2<=5) AND (SELECT count(*) FROM map_tiles b WHERE b.world_id=a.world_id AND (abs(b.q-a.q)+abs(b.q+b.r-a.q-a.r)+abs(b.r-a.r))/2<=2)=19 ORDER BY a.q,a.r LIMIT 1)p"))
  oq,orr=target['q'],target['r']
  sql(f"UPDATE map_tiles SET copper_deposit=false,tin_deposit=false,silver_deposit=false,incense_deposit=false WHERE world_id='{world}'; UPDATE map_tiles SET terrain='hills',silver_deposit=true WHERE world_id='{world}' AND q={oq} AND r={orr};",True)
  def capture(page,code,caption):
   if not code.startswith('D2'):page.wait_for_timeout(300)
   path=OUT/(code+'.png');page.screenshot(path=str(path));shots.append({'code':code,'caption':caption,'file':path.name,'width':page.viewport_size['width']})
   (OUT/'shots.json').write_text(json.dumps(shots,indent=2))
  def low_food():sql(f"UPDATE settlement_goods SET amount=CASE WHEN good_key='grain' THEN 29 WHEN good_key IN('fish','oil','wine','livestock') THEN 0 ELSE 10000 END,rate=0,calc_tick=100 WHERE settlement_id='{me['sid']}';",True)
  with sync_playwright() as pw:
   browser=pw.firefox.launch()
   for mobile,width,height in [(False,1280,900),(True,390,844)]:
    low_food()
    sql(f"DELETE FROM prayer_cooldowns WHERE world_id='{world}' AND player_id='{me['pid']}'; DELETE FROM player_scouted_tiles WHERE world_id='{world}' AND player_id='{me['pid']}' AND (abs(q-{oq})+abs(q+r-{oq}-{orr})+abs(r-{orr}))/2<=2;",True)
    ctx=browser.new_context(viewport={'width':width,'height':height});ctx.add_cookies([{'name':'poleia_token','value':me['token'],'url':base}]);ctx.add_init_script('localStorage.setItem("poleia_token",'+json.dumps(me['token'])+')');ctx.add_init_script(SPY)
    page=ctx.new_page();errors=[];frames=[];page.on('pageerror',lambda e:errors.append(str(e)))
    def watch(ws):
     def received(frame):
      try:
       msg=json.loads(frame)
       if msg.get('kind')=='RiteAnswered':frames.append(msg)
      except (ValueError,TypeError):pass
     ws.on('framereceived',received)
    page.on('websocket',watch)
    page.goto(base+'/world/'+world+'/map',wait_until='networkidle')
    if page.locator('#dc-dismiss-all').is_visible():page.locator('#dc-dismiss-all').click()
    for brief in page.locator('.drawer.open .lb-dismiss').all():
     if brief.is_visible():brief.click()
    page.evaluate("async()=>{window.__proofState=(await import('/static/js/megaron/state.js')).State}")
    # Actual delivered payload, opened using the application's archive renderer.
    page.evaluate("async n=>(await import('/static/js/megaron/ui/dispatch_window.js')).openDispatchWindow(n.kind,n.body_json,(await import('/static/js/megaron/ui/fmt_clock.js')).fmtClock(n.created_at))",notification)
    page.locator('#dw-reply-btn').wait_for();capture(page,'A4' if mobile else 'A1','Myrine’s received letter; Reply opens her letters thread')
    page.locator('#dw-reply-btn').click();page.locator('#diplomacy-body #letter-compose').wait_for()
    capture(page,'A5' if mobile else 'A2','After the actual Reply click: Myrine’s thread and same-runner reply')
    page.evaluate("window.toggleDrawer('diplomacy')")
    page.locator('#trig-kult').click();row='#offer-row-minoan_potnia_vision_v2'
    page.locator('[onclick="okRite(\'minoan_potnia_vision_v2\')"]').click();page.locator(row+' [data-good="grain"]').fill('28')
    line=page.locator(row+' [data-offer-food]');assert "takes 28 of Lato's 29 grain" in line.inner_text() and 'less than Lato eats in a game day' in line.inner_text(),line.inner_text()
    page.evaluate("selector=>document.querySelector(selector).previousElementSibling.scrollIntoView({block:'start'})",row);page.wait_for_timeout(250);capture(page,'A6' if mobile else 'A3','Offer28 of Lato’s29grain; warns that the remaining food is less than one game day’s meal')
    page.evaluate("window.toggleDrawer('kult')");page.wait_for_timeout(250)
    # Use the real march controller and a real POST; no synthetic warning HTML.
    page.evaluate("async a=>{const m=await import('/static/js/megaron/ui/marchctx.js');m.chooseMarchTarget(a.unit);await m.openMarchCtx({q:a.q,r:a.r,known:true,terrain:'plains',isSea:false,isSettlement:false,name:\"Dorian’s Host\"},a.x,85)}",{'unit':troop,'q':q+2,'r':r,'x':180 if mobile else 650})
    with page.expect_response(lambda response:response.request.method=='POST' and response.url.endswith('/march')) as resp:page.locator('#mctx-send').click()
    assert resp.value.status==409,resp.value.text();page.locator('#mctx-hiketeia-go').wait_for();page.wait_for_timeout(250);capture(page,'C2' if mobile else 'C1','Visible risen Host: the server’s warning and March anyway; no order confirmed')
    page.evaluate("window.closeMarchCtx()")
    page.locator('#trig-city').click();food=page.locator('#city-food-sec');page.wait_for_function("document.getElementById('city-food-sec')?.textContent.includes('Storm over 1 fishing ground')");assert 'Storm over 1 fishing ground' in food.inner_text(),food.inner_text();page.wait_for_timeout(350);stormline=food.locator('p').filter(has_text='Storm over 1 fishing ground').first;stormline.evaluate("el=>el.scrollIntoView({block:'center'})");page.wait_for_timeout(350);stormline.scroll_into_view_if_needed();box=stormline.bounding_box();assert box and box['y']>=0 and box['y']+box['height']<height,box;capture(page,'D1m' if mobile else 'D1','Actual storm track over one staffed fishing hex: no fishing while the storm stands')
    page.locator('#drawer-city [data-tab="byggnader"]').click();page.locator('#city-build-select').select_option('pen' if mobile else 'stable')
    with page.expect_response(lambda response:response.request.method=='POST' and response.url.endswith('/build')) as built:page.locator('#city-bld-sec button[onclick="startBuild()"]').click()
    assert built.value.status==201,built.value.text();receipt=page.locator('#city-build-result');page.wait_for_timeout(350)
    assert 'game days' in receipt.inner_text() and 'hours' in receipt.inner_text() and 'ready ' in receipt.inner_text(),receipt.inner_text();receipt.scroll_into_view_if_needed();capture(page,'G2' if mobile else 'G1','Actual build receipt at1800seconds: game days · about real hours · ready in the local clock')
    page.evaluate("window.toggleDrawer('city')")
    # Reset only scene preconditions before the real prayer; no SQL during prayer/reveal.
    sql(f"UPDATE settlement_goods SET amount=1000,rate=0,calc_tick=100 WHERE settlement_id='{me['sid']}' AND good_key IN('grain','silver');",True)
    page.locator('#trig-kult').click();page.locator('[onclick="okRite(\'minoan_potnia_vision_v2\')"]').click()
    # Closing the drawer while the real request runs leaves the map visible for the omen.
    with page.expect_response(lambda response:response.request.method=='POST' and response.url.endswith('/rite')) as answer:
     page.locator(row+' button[onclick="bringOffering(\'minoan_potnia_vision_v2\')"]').click()
    assert answer.value.status==200,answer.value.text()
    page.evaluate("if(document.getElementById('drawer-kult').classList.contains('open'))window.toggleDrawer('kult')")
    page.wait_for_function('window.__proofState.omen && window.__proofState.omen.start>0')
    onset=page.evaluate('window.__proofState.omen.start');timeline=[]
    for i,ms in enumerate((300,1000,2000,4000)):
     page.wait_for_function('(a)=>performance.now()-a.start>=a.ms',arg={'start':onset,'ms':ms})
     code='D2'+('efgh' if mobile else 'abcd')[i]
     actual=page.evaluate('performance.now()')-onset
     capture(page,code,f'Actual WS omen at target{ms/1000:.1f}s; observed pre-screenshot{actual/1000:.3f}s')
     timeline.append({'code':code,'target_ms':ms,'before_screenshot_ms':actual,'after_screenshot_ms':page.evaluate('performance.now()')-onset})
    page.wait_for_function('!window.__proofState.omen');capture(page,'D2j' if mobile else 'D2i','After the complete real animation: the revealed patch is ordinary remembered grey')
    audio=page.evaluate('window.__proofAudio');assert any(abs(x['frequency']-146.8)<.01 for x in audio),audio
    assert frames and frames[-1]['payload'].get('revealed_tiles'),frames
    observations.append({'width':width,'timeline':timeline,'audio':audio,'rite_answered_frames':frames,'errors':errors,'build_receipt':built.value.json()})
    assert not errors,errors
    ctx.close()
   browser.close()
  assert before==hashes(),'production source changed during capture'
  (OUT/'proof.json').write_text(json.dumps({'health':health,'world':world,'tick_seconds':1800,'map_seed':sql(f"SELECT map_seed FROM worlds WHERE id='{world}'"),'source_sha256':before,'actors':[{k:v for k,v in a.items() if k!='token'} for a in (me,writer,fallen)],'site':site,'ore':target,'notification':notification,'observations':observations,'shots':shots},indent=2))
  print('PASS BILD: actual arrival/Reply, offering food warning, march409, storm/fishing, real prayer WS omen+audio observer, build receipt; Firefox1280/390',flush=True)
finally:
 if proc is not None:proc.terminate();proc.wait(timeout=15)
 for name in containers:subprocess.run(['docker','rm','-fv',name],capture_output=True)
