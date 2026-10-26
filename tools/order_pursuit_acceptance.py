#!/usr/bin/env python3
"""Following Runner proof: fresh owned PG/Redis/server and native Firefox.
Usage: python3 tools/order-pursuit_bild_20261026.py OUTPUT
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
 name='megaron-order-pursuit-'+secrets.token_hex(4)+'-'+kind;internal='5432' if kind=='pg' else '6379'
 run('docker','run','-d','--rm','--name',name,'-p','127.0.0.1::'+internal,*args,image);containers.append(name)
 return json.loads(run('docker','inspect',name))[0]['NetworkSettings']['Ports'][internal+'/tcp'][0]['HostPort']
try:
 with tempfile.TemporaryDirectory(prefix='order-pursuit-proof-') as td:
  tmp=Path(td);commit=run('git','-C',str(ROOT),'rev-parse','--short=8','HEAD')+'+order-pursuit'
  for binary,pkg in [('temenos','./cmd/server'),('keryx','./cmd/keryx')]:run('go','build','-ldflags','-X main.buildCommit='+commit,'-o',str(tmp/binary),pkg,cwd=ROOT/'server',env=clean)
  pg=launch('pg','postgres:16-alpine',['-e','POSTGRES_PASSWORD=proof','-e','POSTGRES_DB=proof']);redis=launch('redis','redis:7-alpine',[])
  for _ in range(100):
   if subprocess.run(['docker','exec',containers[0],'pg_isready','-U','postgres','-q'],capture_output=True).returncode==0:break
   time.sleep(.25)
  with socket.socket() as s:s.bind(('127.0.0.1',0));port=s.getsockname()[1]
  base=f'http://127.0.0.1:{port}'
  env={**clean,'DATABASE_URL':f'postgres://postgres:proof@localhost:{pg}/proof?sslmode=disable','REDIS_URL':'localhost:'+redis,'JWT_SECRET':secrets.token_urlsafe(40),'PORT':str(port),'TICK_SECONDS':'10','MAP_WIDTH':'56','MAP_HEIGHT':'40','WORLD_NAME':'March selection proof','POLEIA_WORLD_START_WANAXES':'1','STATIC_DIR':str(ROOT/'web/static'),'TEMPLATE_DIR':str(ROOT/'web/templates'),'CHRONICLE_DIR':str(tmp/'chronicles'),'REPORTS_DIR':str(tmp/'reports')}
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
  site=json.loads(sql(f"SELECT row_to_json(t) FROM(SELECT a.q,a.r FROM map_tiles a WHERE a.world_id='{world}' AND a.q>5 AND a.r>5 AND EXISTS(SELECT 1 FROM map_tiles b WHERE b.world_id=a.world_id AND b.q=a.q+20 AND b.r=a.r) AND NOT EXISTS(SELECT 1 FROM provinces p WHERE p.world_id=a.world_id AND p.map_q=a.q AND p.map_r=a.r) ORDER BY a.q,a.r LIMIT 1)t"));q,r=site['q'],site['r']
  sql(f"UPDATE map_tiles SET terrain='plains',coastal=false WHERE world_id='{world}'; UPDATE provinces SET terrain_type='plains',coastal=false WHERE world_id='{world}'; UPDATE units SET q={q},r={r},status='positioned' WHERE id='{host['id']}';",True)
  home=api(wp+'/founding/settle','POST',{'name':'Lato'},token)['settlement_id']
  sql(f"UPDATE settlements SET population=4000 WHERE id='{home}'; UPDATE settlement_goods SET amount=10000,rate=0,cap=1000000,calc_tick=current_world_tick() WHERE settlement_id='{home}'; UPDATE units SET status='disbanded' WHERE world_id='{world}' AND owner_id='{pid}';",True)
  from datetime import datetime,timezone,timedelta
  uid=sql(f"INSERT INTO units(world_id,owner_id,type,category,size,ordinal,status,q,r,home_settlement_id,support_settlement_id) VALUES('{world}','{pid}','spearman','land',100,5,'positioned',{q+8},{r},'{home}','{home}') RETURNING id").splitlines()[0]
  sql(f"INSERT INTO player_scouted_tiles(world_id,player_id,q,r) SELECT world_id,'{pid}',q,r FROM map_tiles WHERE world_id='{world}' ON CONFLICT DO NOTHING;",True)
  cfg=tmp/'keryx.json';cfg.write_text(json.dumps({'server':base,'token':token,'world_id':world}));cfg.chmod(0o600)
  def cli(*args):
   result=subprocess.run([str(tmp/'keryx'),*args],env={**clean,'POLEIA_CONFIG':str(cfg)},capture_output=True,text=True)
   assert result.returncode==0,(args,result.stderr)
   return result.stdout
  march=json.loads(cli('unit','march','--unit',uid,'--target',f'{q+16},{r}','--json'))
  assert march['following_unit'] and march['status']=='order_dispatched',march
  time.sleep(13) # Earlier march Runner remains in flight; stance departs later.
  stance=json.loads(cli('unit','stance','--unit',uid,'--stance','storm','--json'))
  assert stance['following_unit'] and stance['status']=='order_dispatched',stance
  mid=stance['messenger_id']; snapshots=[]
  def state():return json.loads(sql(f"SELECT json_build_object('unit',(SELECT row_to_json(u) FROM (SELECT id,status,q,r,target_q,target_r,stance,march_route FROM units WHERE id='{uid}')u),'runner',(SELECT row_to_json(m) FROM (SELECT id,status,hex_q,hex_r,dest_q,dest_r,land_route,arrives_at,passage_generation,order_payload FROM messengers WHERE id='{mid}')m))"))
  deadline=time.monotonic()+120
  while time.monotonic()<deadline:
   current=state();snapshots.append(current)
   if current['runner']['order_payload'].get('pursuit_legs',0)>0:break
   assert current['unit']['stance'] is None,'stance applied before Runner caught up'
   time.sleep(.15)
  else:raise AssertionError(('no physical following leg',current))
  assert current['runner']['id']==mid and current['runner']['status']=='outbound',current
  assert current['unit']['stance'] is None,current
  assert current['runner']['land_route']['hexes'][0]==[q+8,r],current
  initial_replanned=current
  roster=api(wp+'/units',token=token)
  (OUT/'replanned-api.json').write_text(json.dumps(roster,indent=2))
  (OUT/'keryx-replanned.txt').write_text(cli('unit','list'))
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
    page.locator('#trig-war').click()
    page.wait_for_function("document.getElementById('war-body').textContent.includes('5th Spearmen')")
    (OUT/f'war-text-{width}.txt').write_text(page.locator('#war-body').inner_text())
    page.wait_for_timeout(500)
    page.screenshot(path=str(OUT/f'following-{width}.png'))
    context.close()
   browser.close()
  assert not errors,errors
  while time.monotonic()<deadline:
   current=state();snapshots.append(current)
   if current['runner']['status']=='arrived':break
   time.sleep(.2)
  else:raise AssertionError(('following never delivered',current))
  assert current['unit']['stance']=='storm',current
  assert source_before==hashes(),'production sources changed during acceptance'
  (OUT/'proof.json').write_text(json.dumps({'health':health,'march':march,'stance':stance,'first_replanned':initial_replanned,'final':current,'snapshots':snapshots,'page_errors':errors,'source_hashes_unchanged':len(source_before),'fixture':'positioned Spearmen; real earlier CLI march and delayed CLI stance, no post-dispatch DB mutations'},indent=2)+'\n')
  print('PASS actual earlier march + delayed stance→same Runner new physical leg→worker meeting; native Firefox1280/390, no post-dispatch fixtures')
finally:
 if proc is not None:
  proc.terminate()
  try:proc.wait(timeout=20)
  except subprocess.TimeoutExpired:proc.kill();proc.wait()
 for name in reversed(containers):subprocess.run(['docker','rm','-f',name],capture_output=True)
