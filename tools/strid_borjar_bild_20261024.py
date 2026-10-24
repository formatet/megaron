#!/usr/bin/env python3
"""Battle-begun proof: fresh owned rig, explicit scheduled-arrival fixture, real worker/WS and Firefox.
Usage: python3 tools/strid_borjar_bild_20261024.py OUTPUT
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
 name='megaron-begun-'+secrets.token_hex(4)+'-'+kind;internal='5432' if kind=='pg' else '6379'
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
  me=actor('Leandros');attacker=actor('Idomeneus')
  site=json.loads(sql(f"SELECT row_to_json(t) FROM(SELECT q,r FROM map_tiles WHERE world_id='{world}' AND terrain='plains' ORDER BY q,r LIMIT 1)t"));q,r=site['q'],site['r']
  sql(f"UPDATE units SET q={q},r={r},status='positioned' WHERE id='{me['host']}'; UPDATE units SET q={q+1},r={r},status='positioned' WHERE id='{attacker['host']}'; UPDATE worlds SET current_tick=100,last_tick_at=now() WHERE id='{world}';",True)
  defender=sql(f"INSERT INTO units(world_id,owner_id,type,category,size,status,q,r) VALUES('{world}','{me['pid']}','spearman','land',100,'positioned',{q},{r}) RETURNING id",True).splitlines()[0]
  army=sql(f"INSERT INTO units(world_id,owner_id,type,category,size,status,q,r,target_q,target_r,capture_mode) VALUES('{world}','{attacker['pid']}','spearman','land',100,'marching',{q+1},{r},{q},{r},'sack') RETURNING id",True).splitlines()[0]
  with sync_playwright() as pw:
   browser=pw.firefox.launch()
   contexts=[]
   for width,height in [(1280,900),(390,844)]:
    ctx=browser.new_context(viewport={'width':width,'height':height});contexts.append(ctx)
    ctx.add_cookies([{'name':'poleia_token','value':me['token'],'url':base}]);ctx.add_init_script('localStorage.setItem("poleia_token",'+json.dumps(me['token'])+')')
    page=ctx.new_page();errors=[];frames=[]
    page.on('pageerror',lambda e:errors.append(str(e)))
    def watch(ws):
     def received(frame):
      try:
       msg=json.loads(frame)
       if msg.get('kind')=='BattleBegun':frames.append(msg)
      except (ValueError,TypeError):pass
     ws.on('framereceived',received)
    page.on('websocket',watch)
    page.goto(base+'/world/'+world+'/map',wait_until='networkidle')
    if page.locator('#dc-dismiss-all').is_visible():page.locator('#dc-dismiss-all').click()
    for brief in page.locator('.drawer.open .lb-dismiss').all():
     if brief.is_visible():brief.click()
    if width==1280:
     sql(f"INSERT INTO scheduled_events(world_id,event_type,payload,process_after,due_tick) VALUES('{world}','UnitArrival','{{\"unit_id\":\"{army}\",\"world_id\":\"{world}\"}}',now(),100);",True)
     for _ in range(150):
      page.wait_for_timeout(200)
      if frames:break
     assert len(frames)==1,frames
     notification=json.loads(sql(f"SELECT row_to_json(n) FROM(SELECT id,kind,level,body_json,created_at,world_tick FROM notifications WHERE world_id='{world}' AND player_id='{me['pid']}' AND kind='BattleBegun')n"))
     assert notification['body_json']['opponent_name']=='Idomeneus',notification
     assert frames[0]['id']==notification['id'],frames
     (OUT/'ws.json').write_text(json.dumps(frames,indent=2))
     # Open the actual live dispatch chip, not a synthetic dialog.
     chip=page.locator('.dispatch-chip').filter(has_text='Idomeneus');chip.wait_for();chip.click()
    else:
     # The mobile archive renderer opens that same persisted payload.
     page.evaluate("async n=>(await import('/static/js/megaron/ui/dispatch_window.js')).openDispatchWindow(n.kind,n.body_json,(await import('/static/js/megaron/ui/fmt_clock.js')).fmtClock(n.created_at))",notification)
    panel=page.locator('#dispatch-window-overlay');panel.wait_for(state='visible');page.wait_for_timeout(350)
    assert 'Idomeneus' in panel.inner_text(),panel.inner_text()
    image=OUT/f'battle-begun-{width}.png';page.screenshot(path=str(image));shots.append({'file':image.name,'width':width,'text':panel.inner_text()})
    assert not errors,errors
    observations.append({'width':width,'page_errors':errors})
   browser.close()
  archive=api(wp+'/notifications',token=me['token']);(OUT/'archive.json').write_text(json.dumps(archive,indent=2))
  assert before==hashes(),'production sources changed during capture'
  (OUT/'proof.json').write_text(json.dumps({'health':health,'world':world,'tick_seconds':1800,'fixture':'Explicit field-army and due-arrival setup; normal UnitArrival worker writes and pushes the notice. No synthetic notice or DOM replacement.','source_sha256':before,'notification':notification,'observations':observations,'shots':shots},indent=2))
  print('PASS: actual UnitArrival worker + BattleBegun WS, persisted archive, live desktop chip, Firefox1280/390',flush=True)
finally:
 if proc is not None:proc.terminate();proc.wait(timeout=15)
 for name in containers:subprocess.run(['docker','rm','-fv',name],capture_output=True)
