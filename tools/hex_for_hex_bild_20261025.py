#!/usr/bin/env python3
"""Timed voyage proof: fresh owned rig, real API and Firefox1280/390.
Usage: python3 tools/hex_for_hex_bild_20261025.py OUTPUT
Explicit scene fixtures are logged. No shared acceptance or production state.
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
def launch(kind,image,args):
 name='megaron-hexphase-'+secrets.token_hex(4)+'-'+kind;internal='5432' if kind=='pg' else '6379'
 run('docker','run','-d','--rm','--name',name,'-p','127.0.0.1::'+internal,*args,image);containers.append(name)
 return json.loads(run('docker','inspect',name))[0]['NetworkSettings']['Ports'][internal+'/tcp'][0]['HostPort']
try:
 with tempfile.TemporaryDirectory(prefix='bildplan-proof-') as td:
  tmp=Path(td);commit=run('git','-C',str(ROOT),'rev-parse','--short=8','HEAD')
  run('go','build','-ldflags','-X main.buildCommit='+commit,'-o',str(tmp/'temenos'),'./cmd/server',cwd=ROOT/'server',env=clean)
  run('go','build','-o',str(tmp/'keryx'),'./cmd/keryx',cwd=ROOT/'server',env=clean)
  pg=launch('pg','postgres:16-alpine',['-e','POSTGRES_PASSWORD=bildplan','-e','POSTGRES_DB=bildplan']);redis=launch('redis','redis:7-alpine',[])
  for _ in range(100):
   if subprocess.run(['docker','exec',containers[0],'pg_isready','-U','postgres','-q'],capture_output=True).returncode==0:break
   time.sleep(.25)
  with socket.socket() as s:s.bind(('127.0.0.1',0));port=s.getsockname()[1]
  base=f'http://127.0.0.1:{port}'
  env={**clean,'DATABASE_URL':f'postgres://postgres:bildplan@localhost:{pg}/bildplan?sslmode=disable','REDIS_URL':'localhost:'+redis,'JWT_SECRET':secrets.token_urlsafe(40),'PORT':str(port),'TICK_SECONDS':'30','MAP_WIDTH':'56','MAP_HEIGHT':'40','WORLD_NAME':'Lato visual proof','POLEIA_WORLD_START_WANAXES':'1','STATIC_DIR':str(ROOT/'web/static'),'TEMPLATE_DIR':str(ROOT/'web/templates'),'CHRONICLE_DIR':str(tmp/'chronicles'),'REPORTS_DIR':str(tmp/'reports')}
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
  assert health['commit']==commit and health['migration']==213,health
  world=api('/api/v1/worlds')[0]['id'];wp='/api/v1/worlds/'+world
  def sql(q,fixture=False):
   if fixture:fixtures.append(q);(OUT/'fixture.sql').write_text('\n\n'.join(fixtures)+'\n')
   return run('docker','exec',containers[0],'psql','-U','postgres','-d','bildplan','-At','-v','ON_ERROR_STOP=1','-c',q)
  token=api('/api/v1/auth/register','POST',{'username':'voyager-'+secrets.token_hex(4),'password':secrets.token_urlsafe(30)})['access_token']
  api(wp+'/join','POST',{},token);pid=api('/api/v1/auth/me',token=token)['id']
  host=next(u for u in api(wp+'/units',token=token)['units'] if u['type']=='nomadic_host')
  site=json.loads(sql(f"SELECT row_to_json(t) FROM(SELECT q,r FROM map_tiles WHERE world_id='{world}' AND terrain='plains' AND q BETWEEN 8 AND 30 AND r BETWEEN 8 AND 25 ORDER BY q,r LIMIT 1)t"));hq,hr=site['q'],site['r']
  sql(f"UPDATE map_tiles SET terrain='plains' WHERE world_id='{world}'; UPDATE map_tiles SET terrain='coastal_sea' WHERE world_id='{world}' AND q BETWEEN {hq+1} AND {hq+4} AND r={hr}; UPDATE units SET q={hq},r={hr},status='positioned' WHERE id='{host['id']}';",True)
  sid=api(wp+'/founding/settle','POST',{'name':'Lato'},token)['settlement_id']
  sql(f"UPDATE settlement_goods SET amount=10000,rate=0,cap=1000000 WHERE settlement_id='{sid}'; DELETE FROM sea_storms WHERE world_id='{world}';",True)
  ship=sql(f"INSERT INTO units(world_id,owner_id,type,category,size,crew,hull,status,settlement_id,support_settlement_id,home_settlement_id,provisions,name) VALUES('{world}','{pid}','galley','naval',1,20,3,'garrison','{sid}','{sid}','{sid}',0,'Sea Palace') RETURNING id;",True).splitlines()[0]
  config=tmp/'keryx.json';config.write_text(json.dumps({'server':base,'token':token,'world_id':world}))
  with sync_playwright() as pw:
   browser=pw.firefox.launch();pages=[]
   for width,height in [(1280,900),(390,844)]:
    ctx=browser.new_context(viewport={'width':width,'height':height})
    ctx.add_cookies([{'name':'poleia_token','value':token,'url':base}]);ctx.add_init_script('localStorage.setItem("poleia_token",'+json.dumps(token)+')')
    page=ctx.new_page();errors=[];page.on('pageerror',lambda e,errors=errors:errors.append(str(e)))
    page.goto(base+'/world/'+world+'/map',wait_until='networkidle')
    if page.locator('#dc-dismiss-all').is_visible():page.locator('#dc-dismiss-all').click()
    for brief in page.locator('.lb-dismiss').all():
     if brief.is_visible():brief.click()
    page.evaluate('(p)=>window.centreOn(...p)',[hq+2,hr]);pages.append((page,width,errors))
   sql(f"UPDATE worlds SET current_tick=103,last_tick_at=now()-interval '22 seconds' WHERE id='{world}'; UPDATE settlement_goods SET calc_tick=103 WHERE settlement_id='{sid}';",True)
   order=api(wp+'/units/'+ship+'/march','POST',{'target_q':hq+4,'target_r':hr,'intent':'patrol'},token)
   (OUT/'order.json').write_text(json.dumps(order,indent=2));start=time.monotonic()
   def capture(code):
    units=api(wp+'/units',token=token);u=next(u for u in units['units'] if u['id']==ship)
    (OUT/(code+'-api.json')).write_text(json.dumps(u,indent=2))
    cli=run(str(tmp/'keryx'),'unit','list',env={**clean,'POLEIA_CONFIG':str(config)});(OUT/(code+'-keryx.txt')).write_text(cli+'\n')
    assert 'Sea Palace' in cli,cli
    for page,width,errors in pages:
     page.evaluate("async(p)=>{const m=await import('/static/js/megaron/render/map.js');await m.loadMap();window.centreOn(...p)}",[hq+2,hr])
     page.wait_for_timeout(150)
     image=OUT/f'{code}-{width}.png';page.screenshot(path=str(image));shots.append({'file':image.name,'code':code,'width':width})
     assert not errors,errors
    observations.append({'phase':code,'elapsed_seconds':time.monotonic()-start,'unit':u,'page_errors':[err for _,_,errors in pages for err in errors]})
    return u
   u=capture('departure');assert u['route_version']==2 and u['current_q']==hq+1 and u['current_r']==hr,u
   while time.monotonic()-start<16:time.sleep(.2)
   u=capture('middle');assert u['status']=='marching' and u['current_q']==hq+2,u
   while time.monotonic()-start<31:time.sleep(.2)
   u=capture('reached-target');assert u.get('current_q',u.get('q'))==hq+4,u
   for _ in range(20):
    u=next(u for u in api(wp+'/units',token=token)['units'] if u['id']==ship)
    if u['status']!='marching':break
    time.sleep(.5)
   assert u['status']=='positioned',u
   capture('completed')
   browser.close()
  assert before==hashes(),'production sources changed during capture'
  (OUT/'proof.json').write_text(json.dumps({'health':health,'source_sha256':before,'source_unchanged':True,'fixture':'Private fresh213 rig; registered/joined/founded through real API. Explicit flat-map/short sea lane, garrisoned ship and22/30-second dispatch phase fixtures; real march API, native Firefox and compiled Keryx. Actual clock, no mocked responses or DOM.','observations':observations,'shots':shots},indent=2))
  print('PASS native Firefox departure/middle/target/completion ×2 widths, real API+Keryx',flush=True)
finally:
 if proc is not None:proc.terminate();proc.wait(timeout=15)
 for name in containers:subprocess.run(['docker','rm','-fv',name],capture_output=True)
