#!/usr/bin/env python3
"""Fresh real game: register → CLI join/found/place → Firefox remove/add.
No SQL writes, FOW fabrication, replay payloads or production endpoints.
"""
from pathlib import Path
import json,os,secrets,socket,subprocess,sys,time,urllib.request
from playwright.sync_api import sync_playwright
HERE=Path(__file__).resolve().parent;ROOT=HERE.parents[2]
OUT=Path(sys.argv[1]).resolve();OUT.mkdir(parents=True,exist_ok=True)
containers=[];proc=None;cli_log=[];errors=[]
clean={k:os.environ[k] for k in ['HOME','PATH']}
def cmd(*args,**kw):return subprocess.check_output(args,text=True,**kw).strip()
def container(service,image,flags):
 name='macchia22-'+service+'-'+secrets.token_hex(4)
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
  if subprocess.run(['docker','exec',containers[0],'pg_isready','-U','postgres','-q'],capture_output=True).returncode==0:break
  time.sleep(.25)
 with socket.socket() as sock:sock.bind(('127.0.0.1',0));port=sock.getsockname()[1]
 base=f'http://127.0.0.1:{port}'
 env=clean|dict(DATABASE_URL=f'postgres://postgres:local-proof-only@127.0.0.1:{pg}/localproof?sslmode=disable',REDIS_URL=f'127.0.0.1:{redis}',JWT_SECRET=secrets.token_urlsafe(40),PORT=str(port),TICK_SECONDS='60',MAP_WIDTH='64',MAP_HEIGHT='64',WORLD_NAME='Sparse timber proof',POLEIA_WORLD_START_WANAXES='1',POLEIA_REGISTRATION_CLOSED='0',STATIC_DIR=str(ROOT/'web/static'),TEMPLATE_DIR=str(ROOT/'web/templates'),CHRONICLE_DIR=str(OUT/'chronicles'),REPORTS_DIR=str(OUT/'reports'))
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
 cli('join');cli('founding','settle','--name','Myrsine','--yes')
 data=json.loads((OUT/'private-player.json').read_text());wid=data['world_id'];pid=data['province_id'];world='/api/v1/worlds/'+wid
 options=api(world+'/provinces/'+pid+'/placement-options',token=token)
 target=next((h for h in options['hexes'] if h['terrain'] in ['scrub_maquis','hills','river_valley'] and any(g['good_key']=='timber' and g['cap']==1 for g in h['goods'])),None)
 assert target,'natural founding site has no new-timber terrain; use a new isolated world, do not edit the map'
 ordinal=target['hex_ordinal'];cli('city');cli('place','timber',str(ordinal),'+1');cli('city');cli('build','--list')
 catalogue=api('/api/v1/buildings',token=token)
 mill=next(b for b in catalogue if b['type']=='lumbermill')
 assert {'scrub_maquis','hills','river_valley'}.issubset(set(mill['requires_terrain']))
 before=api(world+'/provinces/'+pid+'/placement-options',token=token)
 def timber(opts):return next(g for h in opts['hexes'] if h['hex_ordinal']==ordinal for g in h['goods'] if g['good_key']=='timber')
 assert timber(before)['placed']==1 and abs(timber(before)['marginal_yield']-.33)<1e-9
 with sync_playwright() as p:
  b=p.firefox.launch();page=b.new_page(viewport={'width':1280,'height':900},device_scale_factor=1)
  page.on('pageerror',lambda error:errors.append(str(error)))
  page.goto(base+'/',wait_until='domcontentloaded');page.evaluate('t=>localStorage.setItem("poleia_token",t)',token)
  page.context.add_cookies([{'name':'poleia_token','value':token,'url':base}]);page.goto(base+'/play',wait_until='networkidle')
  page.wait_for_function('window.openDrawer !== undefined');page.evaluate("window.openDrawer('city')")
  page.wait_for_selector('#city-gubbe-grid .gubbe-hex-g')
  for brief in page.locator('.lb-dismiss').all():
   if brief.is_visible():brief.click()
  selector='.gubbe-hex-g[data-ordinal="'+str(ordinal)+'"]'
  page.locator(selector).click()
  def row():return page.locator('.gubbe-good-row[data-good="timber"]')
  assert 'timber' in row().inner_text()
  with page.expect_response(lambda r:'/placements/' in r.url and r.request.method=='DELETE') as response:row().locator('[data-verb="unplace1"]').click()
  assert response.value.status==200
  page.wait_for_function('()=>{const r=document.querySelector(".gubbe-good-row[data-good=timber] [data-verb=place1]");return r&&!r.disabled}')
  with page.expect_response(lambda r:r.url.endswith('/placements') and r.request.method=='POST') as response:row().locator('[data-verb="place1"]').click()
  assert response.value.status==201
  page.wait_for_function('()=>{const r=document.querySelector(".gubbe-good-row[data-good=timber] [data-verb=place1]");return r&&r.disabled}')
  for label,width,height in [('desktop',1280,900),('mobile',390,844)]:
   page.set_viewport_size({'width':width,'height':height});row().scroll_into_view_if_needed()
   page.locator('#drawer-city').screenshot(path=str(OUT/('city-'+label+'.png')))
   assert page.locator('#city-body').evaluate('(e)=>e.scrollWidth<=e.clientWidth')
  b.close()
 after=api(world+'/provinces/'+pid+'/placement-options',token=token)
 assert timber(after)['placed']==1 and timber(after)['cap']==1
 assert not errors,errors
 proof=dict(health=health,natural_register_cli_login_join_found=True,world_id=wid,province_id=pid,target={k:target[k] for k in ['terrain','hex_ordinal','hex_q','hex_r']},before=timber(before),after=timber(after),lumbermill_terrains=mill['requires_terrain'],firefox_errors=errors,browser='Firefox; DPR1;1280/390',sql_writes=False,request_replay=False,assets_sha256={p:__import__('hashlib').sha256((ROOT/'web/static'/p).read_bytes()).hexdigest() for p in ['js/megaron/ui/citygrid.js']})
 (OUT/'proof.json').write_text(json.dumps(proof,indent=2)+'\n');(OUT/'cli.json').write_text(json.dumps(cli_log,indent=2)+'\n');print(json.dumps(proof))
finally:
 if proc is not None:proc.terminate();proc.wait(timeout=15)
 for name in reversed(containers):subprocess.run(['docker','rm','-f',name],capture_output=True)
