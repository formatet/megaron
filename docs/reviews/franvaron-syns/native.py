#!/usr/bin/env python3
"""Slice 14: ordinary CLI/API/Firefox flow against isolated local services."""
from pathlib import Path
import json, os, secrets, socket, subprocess, sys, time, urllib.request
from playwright.sync_api import sync_playwright
sys.dont_write_bytecode=True
ROOT=Path(__file__).resolve().parents[3]
OUT=Path(sys.argv[1]).resolve();OUT.mkdir(parents=True,exist_ok=True)
COMMIT=subprocess.check_output(['git','-C',str(ROOT),'rev-parse','HEAD'],text=True).strip()
DIRTY=bool(subprocess.check_output(['git','-C',str(ROOT),'diff','HEAD','--','server','web','tools'],text=True).strip())
containers=[];proc=None;log=None

def command(*args):return subprocess.check_output(args,text=True).strip()
def service(label,image,args,port):
 name='megaron-absence-proof-'+secrets.token_hex(5)+'-'+label
 command('docker','run','-d','--rm','--name',name,'-p','127.0.0.1::'+port,*args,image);containers.append(name)
 return json.loads(command('docker','inspect',name))[0]['NetworkSettings']['Ports'][port+'/tcp'][0]['HostPort']
try:
 pg=service('pg','postgres:16-alpine',['-e','POSTGRES_PASSWORD=proof','-e','POSTGRES_DB=proof'],'5432')
 redis=service('redis','redis:7-alpine',[],'6379')
 for _ in range(100):
  if subprocess.run(['docker','exec',containers[0],'pg_isready','-U','postgres','-q'],capture_output=True).returncode==0:break
  time.sleep(.2)
 with socket.socket() as sock:sock.bind(('127.0.0.1',0));port=sock.getsockname()[1]
 env={k:os.environ[k] for k in ('HOME','PATH')}
 env.update(DATABASE_URL=f'postgres://postgres:proof@127.0.0.1:{pg}/proof?sslmode=disable',REDIS_URL='127.0.0.1:'+redis,JWT_SECRET=secrets.token_urlsafe(32),PORT=str(port),TICK_SECONDS='60',MAP_WIDTH='56',MAP_HEIGHT='40',WORLD_NAME='Absence notice proof',POLEIA_WORLD_START_WANAXES='1',STATIC_DIR=str(ROOT/'web/static'),TEMPLATE_DIR=str(ROOT/'web/templates'),CHRONICLE_DIR=str(OUT/'chronicles'),REPORTS_DIR=str(OUT/'reports'))
 assert not (ROOT/'server/.env').exists()
 for binary,package in [('temenos','./cmd/server'),('keryx','./cmd/keryx')]:subprocess.run(['go','build','-ldflags','-X main.buildCommit='+COMMIT,'-o',str(OUT/binary),package],cwd=ROOT/'server',check=True)
 log=(OUT/'server.log').open('w');proc=subprocess.Popen([str(OUT/'temenos')],cwd=ROOT/'server',env=env,stdout=log,stderr=log)
 base=f'http://127.0.0.1:{port}'
 def api(path,token=None,data=None,method=None):
  req=urllib.request.Request(base+path,data=None if data is None else json.dumps(data).encode(),method=method,headers={'Content-Type':'application/json',**({'Authorization':'Bearer '+token} if token else {})})
  with urllib.request.urlopen(req,timeout=30) as response:return json.load(response)
 for _ in range(180):
  try:health=api('/healthz');break
  except Exception:
   if proc.poll() is not None:raise RuntimeError('server exited')
   time.sleep(.5)
 assert health['commit']==COMMIT and health['migration']==187,health
 username='surface-'+secrets.token_hex(6);password=secrets.token_urlsafe(32)
 api('/api/v1/auth/register',data={'username':username,'password':password})
 config=OUT/'private-config.json'
 envcli={k:os.environ[k] for k in ('HOME','PATH')};envcli.update(POLEIA_CONFIG=str(config),POLEIA_PASSWORD=password)
 def cli(label,*args):
  result=subprocess.run([str(OUT/'keryx'),*args],env=envcli,input='',text=True,capture_output=True)
  text=result.stdout+result.stderr;(OUT/(label+'.txt')).write_text(text);assert result.returncode==0,text
  return text
 cli('login','login','--server',base,'--username',username)
 cli('join','join')
 watch=subprocess.Popen([str(OUT/'keryx'),'watch','--kind','MetropolisFounded','--count','1'],env=envcli,text=True,stdout=subprocess.PIPE,stderr=subprocess.PIPE)
 try:
  time.sleep(.7)
  cli('found','founding','settle','--name','Proofpolis','--yes')
  watchout,watcherr=watch.communicate(timeout=20)
  (OUT/'watch.txt').write_text(watchout+watcherr)
  assert watch.returncode==0 and '[Day #0]' in watchout,watchout+watcherr
 finally:
  if watch.poll() is None:watch.terminate();watch.wait(timeout=5)
 saved=json.loads(config.read_text());token=saved['token'];wp='/api/v1/worlds/'+saved['world_id']
 for label,args in [('status',['status']),('city',['city']),('ticklog',['ticklog'])]:
  text=cli(label,*args);assert 'holds for 144' not in text and 'passive +0.0' not in text,text
 notices=api(wp+'/notifications',token)
 assert notices['notifications'] and all(n.get('world_tick') is not None for n in notices['notifications']),notices
 text=cli('notifications','notifications');assert '[Day #0]' in text and 'holds for 144' not in text,text
 (OUT/'natural-notifications.json').write_text(json.dumps(notices,indent=2))
 errors=[];screens={}
 with sync_playwright() as pw:
  browser=pw.firefox.launch();page=browser.new_page(viewport={'width':1280,'height':900},device_scale_factor=1)
  page.on('pageerror',lambda e:errors.append(str(e)))
  page.goto(base+'/',wait_until='domcontentloaded');page.evaluate('t=>localStorage.setItem("poleia_token",t)',token);page.context.add_cookies([{'name':'poleia_token','value':token,'url':base}]);page.goto(base+'/play',wait_until='networkidle');page.wait_for_function('window.openDrawer !== undefined')
  page.evaluate("openDrawer('notif')");page.wait_for_selector('#notif-body .notif-list-item[data-idx]')
  natural=page.locator('#notif-body').inner_text();assert 'Day #' in natural,natural
  (OUT/'natural-archive.txt').write_text(natural)
  from render import replay_notices
  (OUT/'render').mkdir(exist_ok=True)
  screens=replay_notices(page,OUT/'render')
  browser.close()
 assert not errors,errors
 (OUT/'evidence.json').write_text(json.dumps({'commit':COMMIT,'dirty':DIRTY,'dirty_scope':'tracked server/web/tools versus HEAD','health':health,'proof_scope':'Natural register/login/join/found/CLI/archive dates; explicitly synthetic new-kind renderer replay. Backend transitions verified separately in PostgreSQL.','screens':screens,'page_errors':errors},indent=2))
 print('CLI/API and Firefox 1280/390: GREEN',flush=True)
finally:
 if proc:proc.terminate();proc.wait(timeout=15)
 if log:log.close()
 for name in reversed(containers):subprocess.run(['docker','rm','-fv',name],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
 for name in ('private-config.json','temenos','keryx'):(OUT/name).unlink(missing_ok=True)
