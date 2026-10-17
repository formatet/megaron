#!/usr/bin/env python3
"""Natural fish-fed founding forecast in Firefox/CLI and live welfare; no SQL writes or API overrides."""
from pathlib import Path
import json, os, secrets, socket, subprocess, sys, time, urllib.request
from playwright.sync_api import sync_playwright
ROOT=Path(__file__).resolve().parents[3]
OUT=Path(sys.argv[1]).resolve();OUT.mkdir(parents=True,exist_ok=True)
COMMIT=subprocess.check_output(['git','-C',str(ROOT),'rev-parse','HEAD'],text=True).strip()
containers=[];proc=None

def command(*args): return subprocess.check_output(args,text=True).strip()
def service(label,image,args,port):
 name='megaron-river-'+secrets.token_hex(5)+'-'+label
 command('docker','run','-d','--rm','--name',name,'-p','127.0.0.1::'+port,*args,image);containers.append(name)
 return json.loads(command('docker','inspect',name))[0]['NetworkSettings']['Ports'][port+'/tcp'][0]['HostPort']
try:
 pg=service('pg','postgres:16-alpine',['-e','POSTGRES_PASSWORD=river-proof','-e','POSTGRES_DB=riverproof'],'5432')
 redis=service('redis','redis:7-alpine',[],'6379')
 for _ in range(100):
  if subprocess.run(['docker','exec',containers[0],'pg_isready','-U','postgres','-q'],capture_output=True).returncode==0:break
  time.sleep(.2)
 with socket.socket() as sock:sock.bind(('127.0.0.1',0));port=sock.getsockname()[1]
 env={k:os.environ[k] for k in ('HOME','PATH')}
 env.update(DATABASE_URL=f'postgres://postgres:river-proof@127.0.0.1:{pg}/riverproof?sslmode=disable',REDIS_URL='127.0.0.1:'+redis,JWT_SECRET=secrets.token_urlsafe(32),PORT=str(port),TICK_SECONDS='2',MAP_WIDTH='56',MAP_HEIGHT='40',WORLD_NAME='River proof',POLEIA_WORLD_START_WANAXES='1',STATIC_DIR=str(ROOT/'web/static'),TEMPLATE_DIR=str(ROOT/'web/templates'),CHRONICLE_DIR=str(OUT/'chronicles'),REPORTS_DIR=str(OUT/'reports'))
 assert not (ROOT/'server/.env').exists()
 subprocess.run(['go','build','-ldflags','-X main.buildCommit='+COMMIT,'-o',str(OUT/'temenos'),'./cmd/server'],cwd=ROOT/'server',check=True)
 log=(OUT/'server.log').open('w');proc=subprocess.Popen([str(OUT/'temenos')],cwd=ROOT/'server',env=env,stdout=log,stderr=log)
 base=f'http://127.0.0.1:{port}'
 def api(path,token=None,data=None):
  request=urllib.request.Request(base+path,data=None if data is None else json.dumps(data).encode(),headers={'Content-Type':'application/json',**({'Authorization':'Bearer '+token} if token else {})})
  with urllib.request.urlopen(request,timeout=30) as response:return json.load(response)
 for _ in range(180):
  try:health=api('/healthz');break
  except Exception:
   if proc.poll() is not None:raise RuntimeError('server exited')
   time.sleep(.5)
 assert health['commit']==COMMIT and health['migration']==181,health
 worlds=api('/api/v1/worlds');world=(worlds if isinstance(worlds,list) else worlds['worlds'])[0]['id'];wp='/api/v1/worlds/'+world
 attempts=[]
 for attempt in range(40):
  token=api('/api/v1/auth/register',data={'username':'fish-'+secrets.token_hex(6),'password':secrets.token_urlsafe(32)})['access_token']
  joined=api(wp+'/join',token,{})
  spawn=joined['tile'];q,r=spawn['Q'],spawn['R']
  site=api(wp+f"/colonize-preview?q={q}&r={r}&pop=1000&seed=0&starter_farm=1",token)
  g=site['grain'];attempts.append({'grain':g['base_per_tick'],'fish':g['fish_per_tick'],'net':g['food_net_per_tick']})
  if g['fish_per_tick']>0 and g['food_net_per_tick']>=0 and g['base_per_tick']<5:break
 else:raise AssertionError({'no_fish_fed_spawn':attempts})
 assert not g['loyalty_warning'],g
 (OUT/'forecast.json').write_text(json.dumps(site,indent=2))
 subprocess.run(['go','build','-o',str(OUT/'keryx'),'./cmd/keryx'],cwd=ROOT/'server',check=True)
 config=OUT/'private-config.json';config.write_text(json.dumps({'server':base,'token':token,'world_id':world}));config.chmod(0o600)
 cli_env={k:os.environ[k] for k in ('HOME','PATH')};cli_env['POLEIA_CONFIG']=str(config)
 result=subprocess.run([str(OUT/'keryx'),'founding','settle','--name','Psaropolis'],env=cli_env,input='',text=True,capture_output=True)
 text=result.stdout+result.stderr;(OUT/'forecast-cli.txt').write_text(text)
 assert 'fish ~' in text and 'No grain reserve' not in text and 'loyalty can fall' not in text,text
 errors=[]
 with sync_playwright() as pw:
  browser=pw.firefox.launch();page=browser.new_page(viewport={'width':1280,'height':900},device_scale_factor=1)
  page.on('pageerror',lambda e:errors.append(str(e)))
  page.goto(base+'/',wait_until='domcontentloaded');page.evaluate('t=>localStorage.setItem("poleia_token",t)',token)
  page.context.add_cookies([{'name':'poleia_token','value':token,'url':base}]);page.goto(base+'/play',wait_until='networkidle')
  page.wait_for_function('window.openDrawer !== undefined')
  point=page.evaluate("""async ({q,r})=>{const {State}=await import('/static/js/megaron/state.js');const {hexPx,SCALE}=await import('/static/js/megaron/render/map.js');const p=hexPx(q,r),rect=document.getElementById('hex-canvas').getBoundingClientRect();State.camera.x=rect.width/2-p.x*State.camera.zoom*SCALE;State.camera.y=rect.height/2-p.y*State.camera.zoom*SCALE;State.dirty=true;return {x:rect.left+rect.width/2,y:rect.top+rect.height/2};}""",{'q':q,'r':r})
  page.wait_for_timeout(200);page.mouse.click(point['x'],point['y'])
  page.wait_for_selector('#ip-host-details');page.locator('#ip-host-details summary').click()
  page.wait_for_function("document.getElementById('ip-host-details')?.textContent.includes('consumption')")
  screen=page.locator('#ip-host-details').inner_text();assert 'fish' in screen and 'loyalty can fall' not in screen and 'No grain reserve' not in screen,screen
  (OUT/'forecast-web.txt').write_text(screen);page.screenshot(path=str(OUT/'forecast-firefox.png'))
  page.set_viewport_size({'width':390,'height':844});page.screenshot(path=str(OUT/'forecast-firefox-mobile.png'))
  browser.close()
 founded=api(wp+'/founding/settle',token,{'name':'Psaropolis'})
 province=api(wp+'/provinces/'+founded['province_id'],token)
 settlement=province['settlement'];sid=settlement['id']
 for _ in range(100):
  loyalty=api(wp+'/settlements/'+sid+'/loyalty-log',token)
  rows=loyalty if isinstance(loyalty,list) else loyalty.get('events',loyalty.get('log',[]))
  if any(x.get('event_type')=='well_fed' for x in rows):break
  time.sleep(.25)
 else:raise AssertionError({'no_well_fed':loyalty})
 assert not any(x.get('event_type')=='starving' for x in rows),loyalty
 (OUT/'loyalty.json').write_text(json.dumps(loyalty,indent=2))
 assert not errors,errors
 (OUT/'evidence.json').write_text(json.dumps({'health':health,'attempts':attempts,'spawn':spawn,'founded':founded,'page_errors':errors},indent=2))
 print('natural fish-fed forecast API/CLI/Firefox and FoodTick->well_fed: GREEN',flush=True)
finally:
 if proc:proc.terminate();proc.wait(timeout=15)
 for name in reversed(containers):subprocess.run(['docker','rm','-fv',name],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
