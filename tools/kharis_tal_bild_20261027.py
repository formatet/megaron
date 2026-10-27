#!/usr/bin/env python3
"""Dispatch actions and truthful omen proof: fresh owned PG/Redis/server and native Firefox.
Usage: python3 tools/dispatch_omen_bild_20261026.py OUTPUT
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
 name='megaron-dispatch-omen-'+secrets.token_hex(4)+'-'+kind;internal='5432' if kind=='pg' else '6379'
 run('docker','run','-d','--rm','--name',name,'-p','127.0.0.1::'+internal,*args,image);containers.append(name)
 return json.loads(run('docker','inspect',name))[0]['NetworkSettings']['Ports'][internal+'/tcp'][0]['HostPort']
try:
 with tempfile.TemporaryDirectory(prefix='dispatch-omen-proof-') as td:
  tmp=Path(td);commit=run('git','-C',str(ROOT),'rev-parse','--short=8','HEAD')+'+kharis-tal'
  for binary,pkg in [('temenos','./cmd/server'),('keryx','./cmd/keryx')]:run('go','build','-ldflags','-X main.buildCommit='+commit,'-o',str(tmp/binary),pkg,cwd=ROOT/'server',env=clean)
  pg=launch('pg','postgres:16-alpine',['-e','POSTGRES_PASSWORD=proof','-e','POSTGRES_DB=proof']);redis=launch('redis','redis:7-alpine',[])
  for _ in range(100):
   if subprocess.run(['docker','exec',containers[0],'pg_isready','-U','postgres','-q'],capture_output=True).returncode==0:break
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
  assert health['commit']==commit and health['migration']==217,health
  world=api('/api/v1/worlds')[0]['id'];wp='/api/v1/worlds/'+world
  token=api('/api/v1/auth/register','POST',{'username':'Dispatch-'+secrets.token_hex(4),'password':secrets.token_urlsafe(30)})['access_token']
  api(wp+'/join','POST',{},token);pid=api('/api/v1/auth/me',token=token)['id']
  def sql(q,fixture=False):
   if fixture:fixtures.append(q);(OUT/'fixture.sql').write_text('\n\n'.join(fixtures)+'\n')
   return run('docker','exec',containers[0],'psql','-U','postgres','-d','proof','-At','-v','ON_ERROR_STOP=1','-c',q)
  host=next(u for u in api(wp+'/units',token=token)['units'] if u['type']=='nomadic_host')
  sql(f"UPDATE map_tiles SET terrain='plains',coastal=false WHERE world_id='{world}';",True)
  home=api(wp+'/founding/settle','POST',{'name':'Lato'},token)['settlement_id'];hp=sql(f"SELECT province_id FROM settlements WHERE id='{home}'")
  sql(f"INSERT INTO buildings(settlement_id,building_type,level,god_id) VALUES('{home}','temple',2,'potnia') ON CONFLICT DO NOTHING;",True)
  cfg=tmp/'keryx.json';cfg.write_text(json.dumps({'server':base,'token':token,'world_id':world,'province_id':hp}));cfg.chmod(0o600);clienv={**clean,'POLEIA_CONFIG':str(cfg)}
  with sync_playwright() as pw:
   browser=pw.firefox.launch()
   for cycle in [1,2]:
    for amount,band,chance in [(4,'Wrathful','3%'),(70,'Favorable','15%')]:
     sql(f"UPDATE player_world_records SET kharis_amount={amount},kharis_rate=0,kharis_calc_tick=current_world_tick() WHERE player_id='{pid}' AND world_id='{world}';",True)
     data=api(wp+'/provinces/'+hp,token=token)['settlement'];summary=data['kharis_standing']['summary'];assert band in summary and chance in summary,summary
     profile=api(wp+'/me',token=token);assert profile['kharis_standing']==data['kharis_standing']
     (OUT/f'api-{cycle}-{band}.json').write_text(json.dumps({'province':data,'profile':profile},indent=2))
     for cmd in ['status','me','rite']:
      args=[str(tmp/'keryx'),cmd]+(['--list'] if cmd=='rite' else [])
      result=run(*args,env=clienv);assert summary in result,(cmd,result,summary)
      (OUT/f'keryx-{cycle}-{band}-{cmd}.txt').write_text(result)
     for width,height in [(1280,900),(390,844)]:
      ctx=browser.new_context(viewport={'width':width,'height':height});ctx.add_cookies([{'name':'poleia_token','value':token,'url':base}]);ctx.add_init_script('localStorage.setItem("poleia_token",'+json.dumps(token)+')')
      page=ctx.new_page();errors=[];page.on('pageerror',lambda e:errors.append(str(e)));page.goto(base+'/world/'+world+'/map',wait_until='networkidle')
      if page.locator('#dc-dismiss-all').is_visible():page.locator('#dc-dismiss-all').click()
      for brief in page.locator('.lb-dismiss').all():
       if brief.is_visible():brief.click()
      page.evaluate("async()=>{window.openDrawer('kult');await (await import('/static/js/megaron/ui/drawers/kult.js')).loadKultDrawer()}")
      page.wait_for_timeout(300);text=page.locator('#kult-body').inner_text();assert summary in text,text
      rect=page.locator('#kult-body').evaluate('(e)=>({client:e.clientWidth,scroll:e.scrollWidth})');assert rect['scroll']<=rect['client']+1,rect
      filename=f'{cycle}-{band}-{width}.png';page.screenshot(path=str(OUT/filename));shots.append({'file':filename,'summary':summary});assert not errors,errors
      ctx.close()
   browser.close()
  assert before==hashes(),'production sources changed during capture'
  (OUT/'proof.json').write_text(json.dumps({'health':health,'source_unchanged':True,'shots':shots,'fixture':'Fresh register/join/found; explicit temple2 and kharis4/70 fixtures; real API/Keryx/Firefox, twice each at1280/390. No response or DOM substitution.'},indent=2))
  print('PASS real API/Keryx/Firefox standing, two clean runs at1280/390',flush=True)
finally:
 if proc is not None:proc.terminate();proc.wait(timeout=15)
 for name in containers:subprocess.run(['docker','rm','-fv',name],capture_output=True)
