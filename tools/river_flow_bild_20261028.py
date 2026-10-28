#!/usr/bin/env python3
"""River continuation and direction proof: fresh owned PG/Redis/server and native Firefox.
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
 name='megaron-river-flow-'+secrets.token_hex(4)+'-'+kind;internal='5432' if kind=='pg' else '6379'
 run('docker','run','-d','--rm','--name',name,'-p','127.0.0.1::'+internal,*args,image);containers.append(name)
 return json.loads(run('docker','inspect',name))[0]['NetworkSettings']['Ports'][internal+'/tcp'][0]['HostPort']
try:
 with tempfile.TemporaryDirectory(prefix='wiki-proof-') as td:
  tmp=Path(td);commit=run('git','-C',str(ROOT),'rev-parse','--short=8','HEAD')+'+river-flow'
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
  config.write_text(json.dumps({'server':base,'token':token,'world_id':world,'province_id':hp}))
  sql(f"UPDATE map_tiles SET terrain='coastal_sea',coastal=false WHERE world_id='{world}'; UPDATE map_tiles SET terrain='plains' WHERE world_id='{world}' AND q BETWEEN {hq} AND {hq+12} AND r BETWEEN {hr-3} AND {hr+3}; UPDATE map_tiles SET terrain='river' WHERE world_id='{world}' AND q BETWEEN {hq+5} AND {hq+12} AND r={hr}; UPDATE map_tiles SET terrain='river_ford' WHERE world_id='{world}' AND q={hq+8} AND r={hr}; UPDATE map_tiles SET terrain='coastal_sea' WHERE world_id='{world}' AND q={hq+4} AND r={hr}; UPDATE map_tiles SET terrain='river_delta' WHERE world_id='{world}' AND q={hq+12} AND r={hr+1}; DELETE FROM player_scouted_tiles WHERE world_id='{world}' AND player_id='{pid}'; DELETE FROM player_scouted_provinces WHERE world_id='{world}' AND player_id='{pid}';",True)
  api_proofs=[]
  with sync_playwright() as pw:
   browser=pw.firefox.launch()
   for cycle in [1,2]:
    for code,offset in [('source',5),('fog-edge',8),('mouth',12)]:
     q,r=hq+offset,hr
     known=[(q,r),(q-1,r)]
     if code=='source':known.append((hq+4,hr))
     if code=='mouth':known.extend([(q+1,r),(q+1,r-1),(q,r+1)])
     sql(f"DELETE FROM player_scouted_tiles WHERE world_id='{world}' AND player_id='{pid}'; INSERT INTO player_scouted_tiles(world_id,player_id,q,r) VALUES"+','.join(f"('{world}','{pid}',{x},{y})" for x,y in set(known))+" ON CONFLICT DO NOTHING",True)
     full=api(wp+'/map',token=token);tile=next(t for t in full if t['q']==q and t['r']==r);assert tile['river_flow_known'] and tile['river_downstream_edges'],tile
     if code=='fog-edge':
      hidden=next(t for t in full if t['q']==q+1 and t['r']==r);assert hidden['terrain']=='fog' and not any(k.startswith('river_') for k in hidden),hidden
     api_proofs.append({'cycle':cycle,'scene':code,'tile':tile})
     output=run(str(tmp/'keryx'),'map','--radius','18',env=clienv);assert 'flows ' in output;(OUT/f'{cycle}-{code}-cli.txt').write_text(output)
     for width,height in [(1280,900),(390,844)]:
      ctx,page,errors=browser_page(browser,width,height)
      awaitable="async ([q,r])=>{const m=await import('/static/js/megaron/render/map.js');m.focusDispatchHex(q,r);}"
      page.evaluate(awaitable,[q,r]);page.wait_for_function("document.querySelector('#ip-foot')?.textContent.includes('Flows')")
      text=page.locator('#ip-foot').inner_text();assert 'Flows' in text
      if code=='fog-edge':assert 'Continues into the fog' in text
      page.wait_for_timeout(400);page.screenshot(path=str(OUT/f'{cycle}-{code}-{width}.png'));shots.append({'file':f'{cycle}-{code}-{width}.png'})
      overflow=page.locator('#inspect-panel').evaluate('(e)=>({c:e.clientWidth,s:e.scrollWidth})');assert overflow['s']<=overflow['c']+1,overflow
      assert not errors,errors;ctx.close()
   browser.close()
  assert before==hashes(),'production sources changed during capture'
  (OUT/'proof.json').write_text(json.dumps({'health':health,'shots':shots,'source_unchanged':True,'scenes':api_proofs,'fixture':'Fresh register/join/found; logged source lake, river/ford and sea+delta mouth. Remembered source/middle/mouth windows with actual API FOW; native map panel and Keryx map, twice1280/390. Camera focus uses existing game function, no substituted DOM or API.'},indent=2))
  print('PASS source, fog-edge and mouth API/CLI/native, two desktop/mobile cycles',flush=True)
finally:
 if proc is not None:proc.terminate();proc.wait(timeout=15)
 for name in containers:subprocess.run(['docker','rm','-fv',name],capture_output=True)
