#!/usr/bin/env python3
"""Fresh isolated world: register → CLI login/join/found → real city drawer.
Run twice on separate clean databases. No SQL writes or request replay.
Baseline screenshot serves the baseline renderer and CSS at their actual URLs.
"""
from pathlib import Path
import hashlib,json,os,secrets,socket,subprocess,sys,time,urllib.request
from playwright.sync_api import sync_playwright
HERE=Path(__file__).resolve().parent;ROOT=HERE.parents[2]
OUT=Path(sys.argv[1]).resolve();OUT.mkdir(parents=True,exist_ok=True)
containers=[];proc=None;cli_log=[];errors=[]
clean={k:os.environ[k] for k in ['HOME','PATH']}
def cmd(*args,**kw):return subprocess.check_output(args,text=True,**kw).strip()
def container(service,image,flags):
 name='terrace25-'+service+'-'+secrets.token_hex(4)
 cmd('docker','run','-d','--rm','--name',name,'-p','127.0.0.1::'+('5432' if service=='pg' else '6379'),*flags,image)
 containers.append(name)
 data=json.loads(cmd('docker','inspect',name))[0]['NetworkSettings']['Ports']
 return int(next(iter(data.values()))[0]['HostPort'])
try:
 commit=cmd('git','-C',str(ROOT),'rev-parse','HEAD')
 assert not cmd('git','-C',str(ROOT),'status','--porcelain','--','server'), 'production source dirty'
 subprocess.run(['go','build','-ldflags','-X main.buildCommit='+commit,'-o',str(OUT/'temenos'),'./cmd/server'],cwd=ROOT/'server',env=clean,check=True)
 subprocess.run(['go','build','-o',str(OUT/'keryx'),'./cmd/keryx'],cwd=ROOT/'server',env=clean,check=True)
 pg=container('pg','postgres:16-alpine',['-e','POSTGRES_PASSWORD=local-proof-only','-e','POSTGRES_DB=localproof'])
 redis=container('redis','redis:7-alpine',[])
 for _ in range(60):
  if subprocess.run(['docker','exec',containers[0],'pg_isready','-h','127.0.0.1','-U','postgres','-q'],capture_output=True).returncode==0:break
  time.sleep(.25)
 with socket.socket() as sock:sock.bind(('127.0.0.1',0));port=sock.getsockname()[1]
 base=f'http://127.0.0.1:{port}'
 env=clean|dict(DATABASE_URL=f'postgres://postgres:local-proof-only@127.0.0.1:{pg}/localproof?sslmode=disable',REDIS_URL=f'127.0.0.1:{redis}',JWT_SECRET=secrets.token_urlsafe(40),PORT=str(port),TICK_SECONDS='60',MAP_WIDTH='64',MAP_HEIGHT='64',WORLD_NAME='Terraced city proof',POLEIA_WORLD_START_WANAXES='1',POLEIA_REGISTRATION_CLOSED='0',STATIC_DIR=str(ROOT/'web/static'),TEMPLATE_DIR=str(ROOT/'web/templates'),CHRONICLE_DIR=str(OUT/'chronicles'),REPORTS_DIR=str(OUT/'reports'))
 assert not (ROOT/'server/.env').exists()
 log=(OUT/'server.log').open('w');proc=subprocess.Popen([str(OUT/'temenos')],cwd=ROOT/'server',env=env,stdout=log,stderr=log)
 def api(path,method='GET',body=None,token=None):
  req=urllib.request.Request(base+path,method=method,data=None if body is None else json.dumps(body).encode(),headers={'Content-Type':'application/json',**({'Authorization':'Bearer '+token} if token else {})})
  with urllib.request.urlopen(req,timeout=30) as response:return json.load(response)
 for _ in range(120):
  try:health=api('/healthz');break
  except Exception:
   if proc.poll() is not None:raise RuntimeError('server exited; see server.log')
   time.sleep(.5)
 else:raise RuntimeError('server startup timeout')
 assert health['commit']==commit and health['migration']==189,health
 username='timber-'+secrets.token_hex(4);password=secrets.token_urlsafe(32)
 token=api('/api/v1/auth/register','POST',{'username':username,'password':password})['access_token']
 clienv=clean|dict(POLEIA_CONFIG=str(OUT/'private-player.json'),POLEIA_PASSWORD=password)
 def cli(*args):
  res=subprocess.run([str(OUT/'keryx'),*args],env=clienv,capture_output=True,text=True)
  cli_log.append(dict(args=args,exit_code=res.returncode,stdout=res.stdout,stderr=res.stderr))
  assert res.returncode==0,(args,res.stdout,res.stderr)
  return res.stdout
 cli('login','--server',base,'--username',username)
 cli('join');cli('founding','settle','--name','Myrsine','--yes');cli('city');cli('status')
 data=json.loads((OUT/'private-player.json').read_text());wid=data['world_id'];pid=data['province_id'];world='/api/v1/worlds/'+wid
 payload=api(world+'/provinces/'+pid,token=token)
 (OUT/'province.json').write_text(json.dumps(payload,indent=2)+'\n')
 # Only the two web assets change between the baseline and final browser arms.
 baseline_js=cmd('git','-C',str(ROOT),'show','10e64109:web/static/js/megaron/render/city.js')
 baseline_css=cmd('git','-C',str(ROOT),'show','10e64109:web/static/megaron.css')
 observations=[];asset_proofs=[]
 with sync_playwright() as p:
  b=p.firefox.launch()
  for variant in ['before','after']:
   context=b.new_context(viewport={'width':1280,'height':900},device_scale_factor=1)
   page=context.new_page();page.on('pageerror',lambda error:errors.append(str(error)))
   assets={}
   def record_asset(response):
    for key,path in [('renderer','/static/js/megaron/render/city.js'),('css','/static/megaron.css')]:
     if response.status==200 and response.url.split('?')[0].endswith(path):assets[key]=hashlib.sha256(response.body()).hexdigest()
   page.on('response',record_asset)
   if variant=='before':
    page.route('**/static/js/megaron/render/city.js*',lambda route:route.fulfill(status=200,content_type='text/javascript',body=baseline_js))
    page.route('**/static/megaron.css*',lambda route:route.fulfill(status=200,content_type='text/css',body=baseline_css))
   page.goto(base+'/',wait_until='domcontentloaded');page.evaluate('t=>localStorage.setItem("poleia_token",t)',token)
   context.add_cookies([{'name':'poleia_token','value':token,'url':base}]);page.goto(base+'/play',wait_until='networkidle')
   page.wait_for_function('window.openDrawer !== undefined')
   for brief in page.locator('.lb-dismiss').all():
    if brief.is_visible():brief.click()
   page.evaluate("window.openDrawer('city')")
   page.wait_for_function("()=>document.querySelector('#city-scene')?.width===320 && document.querySelector('#city-food-sec')?.innerText.length>5")
   for brief in page.locator('.lb-dismiss').all():
    if brief.is_visible():brief.click()
   for label,width,height in [('desktop',1280,900),('mobile',390,844)]:
    page.set_viewport_size({'width':width,'height':height})
    panel=page.locator('#drawer-city');canvas=page.locator('#city-scene')
    panel.screenshot(path=str(OUT/('city-'+label+'-'+variant+'.png')))
    canvas.screenshot(path=str(OUT/('canvas-'+label+'-'+variant+'.png')))
    bounds=canvas.evaluate('c=>{const r=c.getBoundingClientRect();return {width:r.width,height:r.height,logical:[c.width,c.height],smoothing:c.getContext("2d").imageSmoothingEnabled}}')
    if variant=='after':assert bounds==dict(width=320,height=192,logical=[320,192],smoothing=False),bounds
    overflow=panel.evaluate('(e)=>e.scrollWidth>e.clientWidth');assert not overflow
    observations.append(dict(variant=variant,width=width,canvas=bounds,panel_overflow=overflow,page_overflow=page.evaluate('document.documentElement.scrollWidth>innerWidth')))
   expected={
    'renderer':hashlib.sha256(baseline_js.encode() if variant=='before' else (ROOT/'web/static/js/megaron/render/city.js').read_bytes()).hexdigest(),
    'css':hashlib.sha256(baseline_css.encode() if variant=='before' else (ROOT/'web/static/megaron.css').read_bytes()).hexdigest()}
   assert assets==expected,(variant,assets,expected)
   asset_proofs.append(dict(variant=variant,sha256=assets,exact_assets_verified=True))
   context.close()
  b.close()
 assert not errors,errors
 proof=dict(health=health,natural_register_cli_login_join_found=True,world_id=wid,province_id=pid,firefox_errors=errors,browser='Firefox; DPR1;1280/390',sql_writes=False,request_replay=False,baseline_assets='10e64109',asset_proofs=asset_proofs,observations=observations)
 (OUT/'proof.json').write_text(json.dumps(proof,indent=2)+'\n');(OUT/'cli.json').write_text(json.dumps(cli_log,indent=2)+'\n');print(json.dumps(proof))
finally:
 if proc is not None:proc.terminate();proc.wait(timeout=15)
 for name in reversed(containers):subprocess.run(['docker','rm','-f',name],capture_output=True)
