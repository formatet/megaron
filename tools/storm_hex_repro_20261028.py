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
  assert health['commit']==commit and health['migration']==220,health
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
  (OUT/'storms-api.json').write_text(json.dumps(api(wp+'/storms',token=me['token']),indent=2))
  observations=[]
  with sync_playwright() as pw:
   browser=pw.firefox.launch()
   for width,height in [(1280,900),(390,844)]:
    ctx=browser.new_context(viewport={'width':width,'height':height});ctx.add_cookies([{'name':'poleia_token','value':me['token'],'url':base}]);ctx.add_init_script('localStorage.setItem("poleia_token",'+json.dumps(me['token'])+')')
    page=ctx.new_page();errors=[];page.on('pageerror',lambda e:errors.append(str(e)));page.goto(base+'/world/'+world+'/map',wait_until='networkidle')
    if page.locator('#dc-dismiss-all').is_visible():page.locator('#dc-dismiss-all').click()
    for el in page.locator('.lb-dismiss').all():
     if el.is_visible():el.click()
    point=page.evaluate("async ([q,r])=>{const m=await import('/static/js/megaron/render/map.js');const {State}=await import('/static/js/megaron/state.js');window.centreOn(q,r);const p=m.hexPx(q,r),b=document.querySelector('canvas').getBoundingClientRect();return {x:b.x+State.camera.x+p.x*State.camera.zoom*m.SCALE,y:b.y+State.camera.y+p.y*State.camera.zoom*m.SCALE};}",[fq,fr])
    page.mouse.click(point['x'],point['y']);page.wait_for_timeout(300)
    text=page.locator('#ip-foot').inner_text();note=page.locator('#ip-foot .storm-note');observations.append({'width':width,'foot':text,'storm_count':note.count(),'visible':note.is_visible() if note.count() else False,'errors':errors})
    page.screenshot(path=str(OUT/f'worked-water-{width}.png'));ctx.close()
   # Open the worked hex while the real storm is elsewhere, then let the normal poll bring it back.
   sql(f"UPDATE map_tiles SET terrain='coastal_sea' WHERE world_id='{world}' AND q={fq+1} AND r={fr+1}; INSERT INTO sea_storm_track(storm_id,tick,slot,q,r) VALUES('{storm}',101,0,{fq+1},{fr}),('{storm}',101,1,{fq},{fr+1}),('{storm}',101,2,{fq+1},{fr+1});",True)
   contexts=[]
   for width,height in [(1280,900),(390,844)]:
    ctx=browser.new_context(viewport={'width':width,'height':height});ctx.add_cookies([{'name':'poleia_token','value':me['token'],'url':base}]);ctx.add_init_script('localStorage.setItem("poleia_token",'+json.dumps(me['token'])+')');page=ctx.new_page();page.goto(base+'/world/'+world+'/map',wait_until='networkidle')
    if page.locator('#dc-dismiss-all').is_visible():page.locator('#dc-dismiss-all').click()
    for el in page.locator('.lb-dismiss').all():
     if el.is_visible():el.click()
    point=page.evaluate("async ([q,r])=>{const m=await import('/static/js/megaron/render/map.js');const {State}=await import('/static/js/megaron/state.js');window.centreOn(q,r);const p=m.hexPx(q,r),b=document.querySelector('canvas').getBoundingClientRect();return {x:b.x+State.camera.x+p.x*State.camera.zoom*m.SCALE,y:b.y+State.camera.y+p.y*State.camera.zoom*m.SCALE};}",[fq,fr]);page.mouse.click(point['x'],point['y']);page.wait_for_timeout(100);assert page.locator('#ip-foot .storm-note').count()==0;contexts.append((ctx,page,width))
   sql(f"INSERT INTO sea_storm_track(storm_id,tick,slot,q,r) VALUES('{storm}',102,0,{fq},{fr}),('{storm}',102,1,{fq+1},{fr}),('{storm}',102,2,{fq},{fr+1});",True)
   for ctx,page,width in contexts:
    page.wait_for_function("async ([q,r])=>{const {State}=await import('/static/js/megaron/state.js');return State.stormData.some(s=>s.hexes.some(h=>h.q===q&&h.r===r));}",arg=[fq,fr],timeout=45000)
    note=page.locator('#ip-foot .storm-note');observations.append({'width':width,'scenario':'storm enters open worked hex','actual_storm_on_hex':True,'storm_count':note.count(),'foot':page.locator('#ip-foot').inner_text()});page.screenshot(path=str(OUT/f'entered-open-hex-{width}.png'));ctx.close()
   browser.close()
  if '--expect-live' in sys.argv:assert all(o['storm_count']==1 for o in observations),observations
  (OUT/'process.json').write_text(json.dumps({'health':health,'source_unchanged':before==hashes()},indent=2))
  (OUT/'observations.json').write_text(json.dumps(observations,indent=2));print(json.dumps(observations),flush=True)
  assert before==hashes(),'production source changed during capture'
finally:
 if proc is not None:proc.terminate();proc.wait(timeout=15)
 for name in containers:subprocess.run(['docker','rm','-fv',name],capture_output=True)
