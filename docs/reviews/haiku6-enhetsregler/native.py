#!/usr/bin/env python3
"""Fresh real game: register → CLI help/Codex/join/found → Firefox Recruit text.
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
 name='haiku24b-'+service+'-'+secrets.token_hex(4)
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
 env=clean|dict(DATABASE_URL=f'postgres://postgres:local-proof-only@127.0.0.1:{pg}/localproof?sslmode=disable',REDIS_URL=f'127.0.0.1:{redis}',JWT_SECRET=secrets.token_urlsafe(40),PORT=str(port),TICK_SECONDS='60',MAP_WIDTH='64',MAP_HEIGHT='64',WORLD_NAME='Cohort help proof',POLEIA_WORLD_START_WANAXES='1',POLEIA_REGISTRATION_CLOSED='0',STATIC_DIR=str(ROOT/'web/static'),TEMPLATE_DIR=str(ROOT/'web/templates'),CHRONICLE_DIR=str(OUT/'chronicles'),REPORTS_DIR=str(OUT/'reports'))
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
 cli('unit','march','--help');cli('unit','reinforce','--help');cli('codex','units');cli('join');cli('founding','settle','--name','Myrsine','--yes');cli('city');cli('status')
 data=json.loads((OUT/'private-player.json').read_text());wid=data['world_id'];pid=data['province_id'];world='/api/v1/worlds/'+wid
 with sync_playwright() as p:
  b=p.firefox.launch();page=b.new_page(viewport={'width':1280,'height':900},device_scale_factor=1)
  page.on('pageerror',lambda error:errors.append(str(error)))
  page.goto(base+'/',wait_until='domcontentloaded');page.evaluate('t=>localStorage.setItem("poleia_token",t)',token)
  page.context.add_cookies([{'name':'poleia_token','value':token,'url':base}]);page.goto(base+'/play',wait_until='networkidle')
  page.wait_for_function('window.openDrawer !== undefined')
  for brief in page.locator('.lb-dismiss').all():
   if brief.is_visible():brief.click()
  observations=[]
  page.evaluate("window.openDrawer('war')")
  page.wait_for_function("()=>document.querySelector('#wtab-recruit').innerText.includes('Land units train')")
  for brief in page.locator('.lb-dismiss').all():
   if brief.is_visible():brief.click()
  page.locator('#war-body [data-tab="recruit"]').click()
  text=page.locator('#wtab-recruit').inner_text()
  assert 'Once ready, a cohort can still march after losing men.' in text
  assert 'only in its original home city' in text
  for label,width,height in [('desktop',1280,900),('mobile',390,844)]:
   page.set_viewport_size({'width':width,'height':height})
   panel=page.locator('#drawer-war');panel.screenshot(path=str(OUT/('recruit-'+label+'.png')))
   overflow=panel.evaluate('(e)=>e.scrollWidth>e.clientWidth');assert not overflow
   observations.append(dict(width=width,overflow=overflow,text=text))
  assert not errors,errors
  b.close()
 proof=dict(health=health,natural_register_cli_login_join_found=True,world_id=wid,province_id=pid,firefox_errors=errors,browser='Firefox; DPR1;1280/390',sql_writes=False,request_replay=False,observations=observations)
 (OUT/'proof.json').write_text(json.dumps(proof,indent=2)+'\n');(OUT/'cli.json').write_text(json.dumps(cli_log,indent=2)+'\n');print(json.dumps(proof))
finally:
 if proc is not None:proc.terminate();proc.wait(timeout=15)
 for name in reversed(containers):subprocess.run(['docker','rm','-f',name],capture_output=True)
