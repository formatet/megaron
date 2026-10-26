#!/usr/bin/env python3
"""Returning expedition redirect proof: fresh owned PG/Redis/server and native Firefox.
Usage: python3 tools/route-sight_bild_20261026.py OUTPUT
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
source_before=hashes();fixtures=[]
(OUT/'source-before.json').write_text(json.dumps(source_before,indent=2))
def launch(kind,image,args):
 name='megaron-route-sight-'+secrets.token_hex(4)+'-'+kind;internal='5432' if kind=='pg' else '6379'
 run('docker','run','-d','--rm','--name',name,'-p','127.0.0.1::'+internal,*args,image);containers.append(name)
 return json.loads(run('docker','inspect',name))[0]['NetworkSettings']['Ports'][internal+'/tcp'][0]['HostPort']
try:
 with tempfile.TemporaryDirectory(prefix='route-sight-proof-') as td:
  tmp=Path(td);commit=run('git','-C',str(ROOT),'rev-parse','--short=8','HEAD')+'+route-sight'
  for binary,pkg in [('temenos','./cmd/server'),('keryx','./cmd/keryx')]:run('go','build','-ldflags','-X main.buildCommit='+commit,'-o',str(tmp/binary),pkg,cwd=ROOT/'server',env=clean)
  pg=launch('pg','postgres:16-alpine',['-e','POSTGRES_PASSWORD=proof','-e','POSTGRES_DB=proof']);redis=launch('redis','redis:7-alpine',[])
  for _ in range(100):
   if subprocess.run(['docker','exec',containers[0],'pg_isready','-h','127.0.0.1','-U','postgres','-q'],capture_output=True).returncode==0:break
   time.sleep(.25)
  with socket.socket() as s:s.bind(('127.0.0.1',0));port=s.getsockname()[1]
  base=f'http://127.0.0.1:{port}'
  env={**clean,'DATABASE_URL':f'postgres://postgres:proof@localhost:{pg}/proof?sslmode=disable','REDIS_URL':'localhost:'+redis,'JWT_SECRET':secrets.token_urlsafe(40),'PORT':str(port),'TICK_SECONDS':'2','MAP_WIDTH':'56','MAP_HEIGHT':'40','WORLD_NAME':'March selection proof','POLEIA_WORLD_START_WANAXES':'1','STATIC_DIR':str(ROOT/'web/static'),'TEMPLATE_DIR':str(ROOT/'web/templates'),'CHRONICLE_DIR':str(tmp/'chronicles'),'REPORTS_DIR':str(tmp/'reports')}
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
  assert health['commit']==commit and health['migration']==217,health
  world=api('/api/v1/worlds')[0]['id'];wp='/api/v1/worlds/'+world
  token=api('/api/v1/auth/register','POST',{'username':'Dispatch-'+secrets.token_hex(4),'password':secrets.token_urlsafe(30)})['access_token']
  api(wp+'/join','POST',{},token);pid=api('/api/v1/auth/me',token=token)['id']
  def sql(q,fixture=False):
   if fixture:fixtures.append(q);(OUT/'fixture.sql').write_text('\n\n'.join(fixtures)+'\n')
   return run('docker','exec',containers[0],'psql','-U','postgres','-d','proof','-At','-v','ON_ERROR_STOP=1','-c',q)
  host=next(u for u in api(wp+'/units',token=token)['units'] if u['type']=='nomadic_host')
  uid=host['id']
  site=json.loads(sql(f"SELECT row_to_json(a) FROM (SELECT q,r FROM map_tiles a WHERE a.world_id='{world}' AND a.q>5 AND a.r>5 AND EXISTS(SELECT 1 FROM map_tiles b WHERE b.world_id=a.world_id AND b.q=a.q+12 AND b.r=a.r) AND EXISTS(SELECT 1 FROM map_tiles b WHERE b.world_id=a.world_id AND b.q=a.q+3 AND b.r=a.r+2) AND EXISTS(SELECT 1 FROM map_tiles b WHERE b.world_id=a.world_id AND b.q=a.q+12 AND b.r=a.r+2) ORDER BY a.q,a.r LIMIT 1) a"))
  q,r=site['q'],site['r']
  sql(f"UPDATE map_tiles SET terrain='plains',coastal=false WHERE world_id='{world}'; UPDATE units SET q={q},r={r},status='positioned' WHERE id='{uid}'; DELETE FROM player_scouted_tiles WHERE world_id='{world}' AND player_id='{pid}'; INSERT INTO player_scouted_tiles(world_id,player_id,q,r) VALUES('{world}','{pid}',{q+8},{r});",True)
  cfg=tmp/'keryx.json';cfg.write_text(json.dumps({'server':base,'token':token,'world_id':world}));cfg.chmod(0o600)
  result=subprocess.run([str(tmp/'keryx'),'unit','march','--unit',uid,'--target',f'{q+8},{r}','--json'],env={**clean,'POLEIA_CONFIG':str(cfg)},capture_output=True,text=True)
  (OUT/'keryx-march.json').write_text(json.dumps({'exit_code':result.returncode,'stdout':result.stdout,'stderr':result.stderr},indent=2))
  assert result.returncode==0,result.stderr
  # No map requests during the journey: only read physical state privately.
  deadline=time.monotonic()+65
  while time.monotonic()<deadline:
   current=json.loads(sql(f"SELECT json_build_object('status',status,'q',q,'r',r) FROM units WHERE id='{uid}'"))
   if current['status']=='positioned' and current['q']==q+8 and current['r']==r:break
   time.sleep(.2)
  else:raise AssertionError(('worker did not arrive',current))
  known=sql(f"SELECT count(*) FROM player_scouted_tiles WHERE world_id='{world}' AND player_id='{pid}' AND q={q+3} AND r={r+2}")
  assert known=='1','intermediate radius missing before first map read'
  data=api(wp+'/map',token=token)
  tiles=data
  middle=next(t for t in tiles if t['q']==q+3 and t['r']==r+2)
  future=next(t for t in tiles if t['q']==q+12 and t['r']==r+2)
  assert middle['tier']=='remembered' and future['tier']=='fog',(middle,future)
  (OUT/'map-proof.json').write_text(json.dumps({'middle':middle,'future':future,'state':current},indent=2))
  cli=subprocess.run([str(tmp/'keryx'),'map','--json'],env={**clean,'POLEIA_CONFIG':str(cfg)},capture_output=True,text=True)
  assert cli.returncode==0,cli.stderr
  (OUT/'keryx-map.json').write_text(cli.stdout)
  errors=[]
  with sync_playwright() as pw:
   browser=pw.firefox.launch()
   for width,height in [(1280,900),(390,844)]:
    context=browser.new_context(viewport={'width':width,'height':height});page=context.new_page();page.on('pageerror',lambda err:errors.append(str(err)))
    page.goto(base);page.evaluate('(token)=>localStorage.setItem("poleia_token",token)',token)
    context.add_cookies([{'name':'poleia_token','value':token,'url':base}])
    page.goto(base+'/world/'+world+'/map',wait_until='networkidle')
    if page.locator('#dc-dismiss-all').is_visible():page.locator('#dc-dismiss-all').click()
    for brief in page.locator('.lb-dismiss').all():
     if brief.is_visible():brief.click()
    page.wait_for_timeout(500)
    page.screenshot(path=str(OUT/f'route-memory-{width}.png'))
    context.close()
   browser.close()
  assert not errors,errors
  assert source_before==hashes(),'production sources changed during acceptance'
  (OUT/'proof.json').write_text(json.dumps({'health':health,'keryx_march':True,'worker_arrival':current,'memory_present_before_first_map_read':True,'middle_tier':middle['tier'],'future_tier':future['tier'],'page_errors':errors,'source_hashes_unchanged':len(source_before),'fixture':'plains map, positioned host, known target only; real CLI order and worker movement'},indent=2)+'\n')
  print('PASS real Keryx→HTTP→worker, no mid-route map reads: intermediate radius remembered, distant future fog, native Firefox desktop/mobile')
finally:
 if proc is not None:
  proc.terminate()
  try:proc.wait(timeout=20)
  except subprocess.TimeoutExpired:proc.kill();proc.wait()
 for name in reversed(containers):subprocess.run(['docker','rm','-f',name],capture_output=True)
