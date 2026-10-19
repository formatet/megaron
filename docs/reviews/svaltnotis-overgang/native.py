#!/usr/bin/env python3
"""Natural hunger/recovery: ordinary register/CLI/founding/placement API, no SQL.
The isolated test world's cadence is 1 second per tick; game rules are unchanged.
A single CLI watch waits for the two real transitions, without polling the API.
"""
from pathlib import Path
import json,os,queue,secrets,socket,subprocess,sys,threading,time,urllib.request
from playwright.sync_api import sync_playwright
HERE=Path(__file__).resolve().parent;ROOT=HERE.parents[2]
OUT=Path(sys.argv[1]).resolve();OUT.mkdir(parents=True,exist_ok=True)
clean={k:os.environ[k] for k in ['HOME','PATH']};containers=[];proc=None;watch=None;calls=[];errors=[]
def command(*args,**kwargs):return subprocess.check_output(args,text=True,**kwargs).strip()
def service(label,image,flags,port):
 name='civil26-'+label+'-'+secrets.token_hex(4)
 command('docker','run','-d','--rm','--name',name,'-p','127.0.0.1::'+str(port),*flags,image);containers.append(name)
 return json.loads(command('docker','inspect',name))[0]['NetworkSettings']['Ports'][str(port)+'/tcp'][0]['HostPort']
try:
 commit=command('git','-C',str(ROOT),'rev-parse','HEAD');assert not command('git','-C',str(ROOT),'status','--porcelain','--','server','web')
 for name,path in [('temenos','./cmd/server'),('keryx','./cmd/keryx')]:
  subprocess.run(['go','build','-ldflags','-X main.buildCommit='+commit,'-o',str(OUT/name),path],cwd=ROOT/'server',env=clean,check=True)
 pg=service('pg','postgres:16-alpine',['-e','POSTGRES_PASSWORD=local-proof-only','-e','POSTGRES_DB=proof'],5432)
 redis=service('redis','redis:7-alpine',[],6379)
 for _ in range(100):
  if subprocess.run(['docker','exec',containers[0],'pg_isready','-h','127.0.0.1','-U','postgres','-q'],capture_output=True).returncode==0:break
  time.sleep(.2)
 with socket.socket() as sock:sock.bind(('127.0.0.1',0));port=sock.getsockname()[1]
 base=f'http://127.0.0.1:{port}'
 env=clean|dict(DATABASE_URL=f'postgres://postgres:local-proof-only@127.0.0.1:{pg}/proof?sslmode=disable',REDIS_URL='127.0.0.1:'+redis,JWT_SECRET=secrets.token_urlsafe(32),PORT=str(port),TICK_SECONDS='1',MAP_WIDTH='64',MAP_HEIGHT='64',WORLD_NAME='Civilian hunger proof',POLEIA_WORLD_START_WANAXES='1',POLEIA_REGISTRATION_CLOSED='0',STATIC_DIR=str(ROOT/'web/static'),TEMPLATE_DIR=str(ROOT/'web/templates'),CHRONICLE_DIR=str(OUT/'chronicles'),REPORTS_DIR=str(OUT/'reports'))
 assert not (ROOT/'server/.env').exists()
 log=(OUT/'server.log').open('w');proc=subprocess.Popen([str(OUT/'temenos')],cwd=ROOT/'server',env=env,stdout=log,stderr=log)
 def api(path,token=None,method='GET',body=None):
  req=urllib.request.Request(base+path,method=method,data=None if body is None else json.dumps(body).encode(),headers={'Content-Type':'application/json',**({'Authorization':'Bearer '+token} if token else {})})
  with urllib.request.urlopen(req,timeout=30) as response:return json.load(response)
 for _ in range(150):
  try:health=api('/healthz');break
  except Exception:
   if proc.poll() is not None:raise RuntimeError('server exited')
   time.sleep(.25)
 else:raise RuntimeError('server startup timed out')
 assert health['commit']==commit and health['migration']==189,health
 username='civil-'+secrets.token_hex(4);password=secrets.token_urlsafe(30)
 token=api('/api/v1/auth/register',method='POST',body={'username':username,'password':password})['access_token']
 cli_env=clean|dict(POLEIA_CONFIG=str(OUT/'private-player.json'),POLEIA_PASSWORD=password)
 def cli(*args):
  r=subprocess.run([str(OUT/'keryx'),*args],env=cli_env,capture_output=True,text=True,timeout=30)
  calls.append(dict(args=args,exit_code=r.returncode,stdout=r.stdout,stderr=r.stderr));assert r.returncode==0,(args,r.stdout,r.stderr)
  return r.stdout
 cli('login','--server',base,'--username',username);cli('join');cli('founding','settle','--name','Myrsine','--yes');cli('status')
 config=json.loads((OUT/'private-player.json').read_text());world='/api/v1/worlds/'+config['world_id'];pid=config['province_id'];placement_path=world+'/provinces/'+pid+'/placements'
 original=api(placement_path,token)['placements'];assert original
 (OUT/'initial-placements.json').write_text(json.dumps(original,indent=2)+'\n')
 watch=subprocess.Popen([str(OUT/'keryx'),'watch','--kind','CivilianStarvationStarted,CivilianStarvationEnded','--count','2'],env=cli_env,text=True,stdout=subprocess.PIPE,stderr=subprocess.PIPE)
 lines=[];messages=queue.Queue()
 def pump():
  for line in watch.stdout:lines.append(line);messages.put(line)
  messages.put(None)
 thread=threading.Thread(target=pump,daemon=True);thread.start()
 def await_notice(kind,budget=120):
  deadline=time.monotonic()+budget
  while time.monotonic()<deadline:
   try:line=messages.get(timeout=max(.1,deadline-time.monotonic()))
   except queue.Empty:break
   if line is None:raise RuntimeError('watch ended before '+kind+': '+''.join(lines))
   if kind in line:return
  raise RuntimeError('no natural '+kind+' within '+str(budget)+'s; see server/watch log')
 for row in original:api(placement_path+'/'+str(row['gubbe_ordinal']),token,method='DELETE')
 # Derive the wait from the real food basket instead of assuming a starter stock.
 initial=api(world+'/provinces/'+pid,token)['settlement']
 demand=initial['grain_consum_rate'];drain=-initial['food']['food_net_per_tick']
 assert drain>0,initial['food']
 budget=int(initial['food']['coverage_ticks']*demand/drain)+91
 (OUT/'withdrawn-food-state.json').write_text(json.dumps(initial,indent=2)+'\n')
 await_notice('CivilianStarvationStarted',budget)
 hungry=api(world+'/notifications',token);(OUT/'hungry-notifications.json').write_text(json.dumps(hungry,indent=2)+'\n')
 hungry_cli=cli('notifications');assert 'CivilianStarvationStarted' in hungry_cli and 'people are going hungry' in hungry_cli
 observations=[]
 with sync_playwright() as p:
  browser=p.firefox.launch();page=browser.new_page(viewport={'width':1280,'height':900},device_scale_factor=1)
  page.on('pageerror',lambda error:errors.append(str(error)))
  page.goto(base+'/',wait_until='domcontentloaded');page.evaluate('t=>localStorage.setItem("poleia_token",t)',token);page.context.add_cookies([{'name':'poleia_token','value':token,'url':base}]);page.goto(base+'/play',wait_until='networkidle')
  page.wait_for_function('window.openDrawer !== undefined')
  for brief in page.locator('.lb-dismiss').all():
   if brief.is_visible():brief.click()
  page.evaluate("window.openDrawer('notif')")
  page.wait_for_function("()=>document.getElementById('notif-body')?.innerText.includes('people are going hungry')")
  for label,width,height in [('desktop',1280,900),('mobile',390,844)]:
   page.set_viewport_size({'width':width,'height':height});panel=page.locator('#drawer-notif');panel.screenshot(path=str(OUT/('hungry-'+label+'.png')))
   text=page.locator('#notif-body').inner_text();assert text.index('people are going hungry')<text.index('metropolis'),text
   assert not panel.evaluate('e=>e.scrollWidth>e.clientWidth')
   observations.append(dict(phase='hungry',width=width,text=text,overflow=False))
  for row in original:
   body={k:row[k] for k in ['target_kind','hex_ordinal','building_type','good_key'] if k in row}
   api(placement_path,token,method='POST',body=body)
  await_notice('CivilianStarvationEnded')
  recovered=api(world+'/notifications',token);(OUT/'recovered-notifications.json').write_text(json.dumps(recovered,indent=2)+'\n')
  text=cli('notifications');assert 'people were fed again' in text
  codex=cli('codex','food');assert 'rather than repeating the warning every day' in codex
  page.evaluate("window.closeDrawer('notif');window.openDrawer('notif')")
  page.wait_for_function("()=>document.getElementById('notif-body')?.innerText.includes('people were fed again')")
  for label,width,height in [('desktop',1280,900),('mobile',390,844)]:
   page.set_viewport_size({'width':width,'height':height});panel=page.locator('#drawer-notif');panel.screenshot(path=str(OUT/('recovered-'+label+'.png')))
   assert not panel.evaluate('e=>e.scrollWidth>e.clientWidth')
   observations.append(dict(phase='recovered',width=width,text=page.locator('#notif-body').inner_text(),overflow=False))
  browser.close()
 watch.wait(timeout=15);thread.join(timeout=5);watch_err=watch.stderr.read();(OUT/'watch.txt').write_text(''.join(lines)+watch_err);assert watch.returncode==0
 start=[n for n in recovered['notifications'] if n['kind']=='CivilianStarvationStarted'];end=[n for n in recovered['notifications'] if n['kind']=='CivilianStarvationEnded']
 assert len(start)==1 and len(end)==1 and start[0]['world_tick']<end[0]['world_tick']
 assert not any(n['kind']=='FoodShortfall' for n in recovered['notifications'])
 assert not errors,errors
 proof=dict(health=health,world_id=config['world_id'],province_id=pid,natural_register_cli_join_found_unplace_hunger_restore_recovery=True,sql_writes=False,api_replay=False,server_cadence_seconds=1,firefox='DPR1;1280/390',page_errors=errors,start_tick=start[0]['world_tick'],end_tick=end[0]['world_tick'],missing_grain_equivalent=start[0]['body']['unmet'],observations=observations)
 (OUT/'proof.json').write_text(json.dumps(proof,indent=2)+'\n');(OUT/'cli.json').write_text(json.dumps(calls,indent=2)+'\n');print(json.dumps(proof),flush=True)
finally:
 if watch is not None and watch.poll() is None:watch.terminate();watch.wait(timeout=10)
 if proc is not None:proc.terminate();proc.wait(timeout=15)
 for name in reversed(containers):subprocess.run(['docker','rm','-f',name],capture_output=True)
 (OUT/'private-player.json').unlink(missing_ok=True)
