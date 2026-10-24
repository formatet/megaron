#!/usr/bin/env python3
"""D1/D2 correction proof: owned fresh runtime, actual prayer WS, normal fog and Firefox.
Usage: python3 tools/omen_dimma_bild_20261024.py OUTPUT
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
 name='megaron-omenfix-'+secrets.token_hex(4)+'-'+kind;internal='5432' if kind=='pg' else '6379'
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
   path=OUT/(code+'.png');page.screenshot(path=str(path));shots.append({'code':code,'caption':caption,'file':path.name,'width':page.viewport_size['width']});(OUT/'shots.json').write_text(json.dumps(shots,indent=2))
  with sync_playwright() as pw:
   browser=pw.firefox.launch()
   for mobile,width,height in [(False,1280,900),(True,390,844)]:
    # One already remembered tile inside the prayer patch must never reanimate.
    sql(f"DELETE FROM prayer_cooldowns WHERE world_id='{world}' AND player_id='{me['pid']}'; DELETE FROM player_scouted_tiles WHERE world_id='{world}' AND player_id='{me['pid']}' AND (abs(q-{oq})+abs(q+r-{oq}-{orr})+abs(r-{orr}))/2<=2; INSERT INTO player_scouted_tiles(world_id,player_id,q,r) VALUES('{world}','{me['pid']}',{oq+1},{orr}) ON CONFLICT DO NOTHING; UPDATE settlement_goods SET amount=10000,rate=0,calc_tick=100 WHERE settlement_id='{me['sid']}' AND good_key IN('grain','silver'); UPDATE player_world_records SET kharis_amount=90,kharis_calc_tick=100 WHERE player_id='{me['pid']}' AND world_id='{world}';",True)
    ctx=browser.new_context(viewport={'width':width,'height':height});ctx.add_cookies([{'name':'poleia_token','value':me['token'],'url':base}]);ctx.add_init_script('localStorage.setItem("poleia_token",'+json.dumps(me['token'])+')')
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
    page.evaluate("async()=>{window.__proofState=(await import('/static/js/megaron/state.js')).State;window.__proofMap=await import('/static/js/megaron/render/map.js')}")
    page.locator('#trig-city').click();page.wait_for_function("document.getElementById('city-food-sec')?.textContent.includes('its fishers stay but catch nothing')");page.wait_for_timeout(350)
    line=page.locator('#city-food-sec p').filter(has_text='Storm over 1 fishing ground').first;line.evaluate("el=>el.scrollIntoView({block:'center'})");page.wait_for_timeout(300);line.scroll_into_view_if_needed();capture(page,f'D1-{width}','Fishers stay placed and catch nothing while the storm covers their fishing ground')
    page.evaluate("window.toggleDrawer('city')");page.wait_for_timeout(350)
    page.evaluate("a=>{const s=window.__proofState,m=window.__proofMap;if(s.camera.zoom!==1)m.zoom(1/s.camera.zoom);window.centreOn(a.q,a.r)}",{'q':oq,'r':orr})
    page.wait_for_timeout(700);page.wait_for_function('window.__proofMap.markrasterStats.pending===0')
    beforeCanvas=page.locator('#hex-canvas').evaluate('el=>el.toDataURL()');capture(page,f'D2-before-{width}','Before the actual prayer: ordinary striped fog, with one remembered tile already known')
    beforeTiles=page.evaluate('JSON.parse(JSON.stringify(window.__proofState.tileData))')
    response=api(wp+'/settlements/'+me['sid']+'/rite','POST',{'prayer':'minoan_potnia_vision_v2'},me['token'])
    page.wait_for_function('window.__proofState.omen && Number.isFinite(window.__proofState.omen.start)')
    onset=page.evaluate('window.__proofState.omen.start');zeroElapsed=page.evaluate('performance.now()')-onset
    zeroCanvas=page.locator('#hex-canvas').evaluate('el=>el.toDataURL()')
    assert zeroElapsed<300,zeroElapsed
    assert beforeCanvas==zeroCanvas,'zero-phase canvas differs from the map before prayer'
    capture(page,f'D2-zero-{width}','Actual WS, before the first 300ms step: canvas is byte-identical to before the prayer')
    initial=page.evaluate('window.__proofState.omen.tiles')
    old={(t['q'],t['r']):t for t in beforeTiles}
    assert all((t['q'],t['r']) not in old or old[(t['q'],t['r'])]['terrain']=='fog' for t in initial),initial
    assert {'q':oq+1,'r':orr} not in initial,initial
    timeline=[]
    for ms in (300,1000,2000,4000):
     page.wait_for_function('(a)=>performance.now()-a.start>=a.ms',arg={'start':onset,'ms':ms})
     actual=page.evaluate('performance.now()')-onset;capture(page,f'D2-{ms}-{width}',f'Actual animation at target {ms/1000:g}s; pre-screenshot {actual/1000:.3f}s')
     timeline.append({'target_ms':ms,'before_screenshot_ms':actual,'after_screenshot_ms':page.evaluate('performance.now()')-onset})
    page.wait_for_function('!window.__proofState.omen');page.wait_for_function('window.__proofMap.markrasterStats.pending===0');capture(page,f'D2-memory-{width}','Finished: ordinary remembered terrain, no replacement fog')
    assert frames and frames[-1]['payload'].get('revealed_tiles'),frames
    assert not errors,errors
    observations.append({'width':width,'zero_canvas_byte_equal':beforeCanvas==zeroCanvas,'zero_elapsed_ms':zeroElapsed,'eligible_tiles':initial,'server_revealed_count':len(frames[-1]['payload']['revealed_tiles']),'before_tiles':beforeTiles,'timeline':timeline,'rite_answered_frames':frames,'response':response,'errors':errors})
    ctx.close()
   browser.close()
  assert before==hashes(),'production sources changed during capture'
  (OUT/'proof.json').write_text(json.dumps({'health':health,'world':world,'tick_seconds':1800,'source_sha256':before,'site':site,'ore':target,'observations':observations,'shots':shots},indent=2))
  print('PASS: D1 fishers stay; actual prayer WS; original fog; before/zero canvas byte equal; only unknown tiles; Firefox1280/390',flush=True)
finally:
 if proc is not None:proc.terminate();proc.wait(timeout=15)
 for name in containers:subprocess.run(['docker','rm','-fv',name],capture_output=True)
