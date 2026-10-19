#!/usr/bin/env python3
"""Natural four-food surfaces and mixed provisions through a real patrol. No SQL writes."""
from pathlib import Path
import json,os,queue,secrets,socket,subprocess,sys,threading,time,urllib.request
from playwright.sync_api import sync_playwright, Error as BrowserError
from PIL import Image
HERE=Path(__file__).resolve().parent;ROOT=HERE.parents[2]
OUT=Path(sys.argv[1]).resolve();OUT.mkdir(parents=True,exist_ok=True)
clean={k:os.environ[k] for k in ['HOME','PATH']};containers=[];proc=None;watch=None;calls=[];errors=[]
def command(*args,**kwargs):return subprocess.check_output(args,text=True,**kwargs).strip()
def service(label,image,flags,port):
 name='mat28-'+label+'-'+secrets.token_hex(4)
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
 env=clean|dict(DATABASE_URL=f'postgres://postgres:local-proof-only@127.0.0.1:{pg}/proof?sslmode=disable',REDIS_URL='127.0.0.1:'+redis,JWT_SECRET=secrets.token_urlsafe(32),PORT=str(port),TICK_SECONDS='1',MAP_WIDTH='64',MAP_HEIGHT='64',WORLD_NAME='Equal food proof',POLEIA_WORLD_START_WANAXES='1',POLEIA_REGISTRATION_CLOSED='0',STATIC_DIR=str(ROOT/'web/static'),TEMPLATE_DIR=str(ROOT/'web/templates'),CHRONICLE_DIR=str(OUT/'chronicles'),REPORTS_DIR=str(OUT/'reports'))
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
 assert health['commit']==commit and health['migration']==190,health
 username='food-'+secrets.token_hex(4);password=secrets.token_urlsafe(30)
 token=api('/api/v1/auth/register',method='POST',body={'username':username,'password':password})['access_token']
 cli_env=clean|dict(POLEIA_CONFIG=str(OUT/'private-player.json'),POLEIA_PASSWORD=password)
 def cli(*args):
  r=subprocess.run([str(OUT/'keryx'),*args],env=cli_env,capture_output=True,text=True,timeout=30)
  calls.append(dict(args=args,exit_code=r.returncode,stdout=r.stdout,stderr=r.stderr));assert r.returncode==0,(args,r.stdout,r.stderr)
  return r.stdout
 cli('login','--server',base,'--username',username);cli('join')
 config=json.loads((OUT/'private-player.json').read_text());world='/api/v1/worlds/'+config['world_id']
 land={'plains','hills','forest_olive_grove','forest_cedar','scrub_maquis','river_valley','river_delta'}
 journey=[];visited=set()
 def axial(a,b):return (abs(a['q']-b['q'])+abs(a['r']-b['r'])+abs(a['q']-b['q']+a['r']-b['r']))/2
 for leg in range(8):
  fp=api(world+'/founding/status',token);known=api(world+'/map',token);visited.add((fp['q'],fp['r']))
  candidates=[t for t in known if t['visible'] and t.get('coastal') and t['terrain'] in land]
  if candidates:
   coast=min(candidates,key=lambda t:axial(t,fp))
  else:
   frontier=[t for t in known if t['visible'] and t['terrain'] in land and (t['q'],t['r']) not in visited]
   assert frontier,'no known land route toward a coast'
   fog=[t for t in known if not t['visible']]
   coast=max(frontier,key=lambda t:(sum(axial(t,f)<=2 for f in fog),axial(t,fp)))
  journey.append(dict(from_q=fp['q'],from_r=fp['r'],target_q=coast['q'],target_r=coast['r'],known_coast=bool(candidates)))
  (OUT/'host-journey.json').write_text(json.dumps(journey,indent=2)+'\n')
  if (fp['q'],fp['r'])!=(coast['q'],coast['r']):
   cli('unit','march','--unit',fp['host_unit_id'],'--q',str(coast['q']),'--r',str(coast['r']),'--yes')
   for _ in range(180):
    moved=api(world+'/founding/status',token)
    if (moved['q'],moved['r'])==(coast['q'],coast['r']):break
    time.sleep(.5)
   else:raise RuntimeError('host did not reach ordinary known land')
  if candidates:break
 else:raise RuntimeError('no coast found within bounded ordinary host exploration')
 founded=json.loads(cli('--json','founding','settle','--name','Myrsine','--yes'))
 config=json.loads((OUT/'private-player.json').read_text());world='/api/v1/worlds/'+config['world_id'];pid=config['province_id']
 (OUT/'founding.json').write_text(json.dumps(founded,indent=2)+'\n')
 def snapshot(label):
  data=api(world+'/provinces/'+pid,token);(OUT/(label+'.json')).write_text(json.dumps(data,indent=2)+'\n');return data
 # Bounded readiness wait for an actual meal; no DB writes or simulated payload.
 for _ in range(80):
  city=snapshot('meal')
  if city['settlement']['food'].get('consumed',{}).get('fish',0)>0:break
  time.sleep(.25)
 else:raise RuntimeError('no natural mixed meal at this founding site')
 food=city['settlement']['food'];assert food['consumed']['grain']>0 and food['consumed_tick'] is not None,food
 assert set(food['goods'])=={'grain','fish','oil','wine'},food
 status=cli('status');cli('city');cli('unit','list');cli('codex','food');cli('codex','sea')
 assert 'Food for ' in status and all(g in status for g in ['grain','fish','oil','wine']),status
 roster=api(world+'/units',token)['units'];(OUT/'units-before.json').write_text(json.dumps(roster,indent=2)+'\n')
 ships=[u for u in roster if u['category']=='naval'];assert ships,'no coastal gift at this natural founding'
 ship=ships[0];sid=ship['id']
 tiles=api(world+'/map',token)
 origin=founded['tile'];q=origin.get('q',origin.get('Q'));r=origin.get('r',origin.get('R'))
 def distance(t):return (abs(t['q']-q)+abs(t['r']-r)+abs(t['q']-q+t['r']-r))/2
 target=min([t for t in tiles if t['visible'] and t['terrain']=='coastal_sea' and distance(t)>=2],key=distance)
 cli('unit','march','--unit',sid,'--q',str(target['q']),'--r',str(target['r']),'--intent','patrol','--preview')
 cli('unit','march','--unit',sid,'--q',str(target['q']),'--r',str(target['r']),'--intent','patrol','--yes')
 loaded=next(u for u in api(world+'/units',token)['units'] if u['id']==sid)
 (OUT/'units-loaded.json').write_text(json.dumps(loaded,indent=2)+'\n')
 assert loaded['provisions_by_good']['grain']>0 and loaded['provisions_by_good']['fish']>0,loaded
 cli('unit','list')
 observations=[]
 with sync_playwright() as p:
  browser=p.firefox.launch();page=browser.new_page(viewport={'width':1280,'height':900},device_scale_factor=1)
  page.on('pageerror',lambda error:errors.append(str(error)))
  page.goto(base+'/',wait_until='domcontentloaded');page.evaluate('t=>localStorage.setItem("poleia_token",t)',token);page.context.add_cookies([{'name':'poleia_token','value':token,'url':base}]);page.goto(base+'/play',wait_until='networkidle')
  page.wait_for_function('window.openDrawer !== undefined')
  for brief in page.locator('.lb-dismiss').all():
   if brief.is_visible():brief.click()
  page.evaluate("window.openDrawer('city')");page.wait_for_function("()=>document.getElementById('city-food-sec')?.innerText.includes('Food for')")
  for label,width,height in [('desktop',1280,900),('mobile',390,844)]:
   page.set_viewport_size({'width':width,'height':height})
   page.wait_for_function("()=>['grain','fish','oil','wine'].every(g=>document.getElementById('city-food-sec')?.innerText.includes(g))")
   page.locator('#city-food-sec').scroll_into_view_if_needed();panel=page.locator('#drawer-city');panel.screenshot(path=str(OUT/('food-'+label+'.png')))
   page.wait_for_function("()=>['grain','fish','oil','wine'].every(g=>document.getElementById('city-food-sec')?.innerText.includes(g))")
   text=page.locator('#city-food-sec').inner_text();assert all(g in text for g in ['grain','fish','oil','wine']),text
   # The drawer refreshes on world ticks. Capture the real row itself and
   # reject a blank transitional frame (pixel count is presence, not legibility).
   for capture_attempt in range(5):
    page.wait_for_function("()=>['grain','fish','oil','wine'].every(g=>document.getElementById('city-food-sec')?.innerText.includes(g))")
    row_path=OUT/('food-row-'+label+'.png')
    try:page.locator('#city-food-sec').screenshot(path=str(row_path),timeout=5000)
    except BrowserError as capture_error:
     if 'not attached to the DOM' in str(capture_error):continue
     raise
    with Image.open(row_path) as shot:ink=sum(1 for px in shot.convert('RGB').getdata() if px[0]<190 and px[1]<150 and px[2]<150)
    if ink>40:break
   else:raise RuntimeError('food row capture remained blank')
   assert not panel.evaluate('e=>e.scrollWidth>e.clientWidth')
   observations.append(dict(drawer='city',width=width,text=text,overflow=False))
  page.evaluate("window.closeDrawer('city');window.openDrawer('war')")
  page.wait_for_function("()=>document.getElementById('drawer-war')?.innerText.includes('food')")
  for label,width,height in [('desktop',1280,900),('mobile',390,844)]:
   page.set_viewport_size({'width':width,'height':height});page.locator('#drawer-war').get_by_text('food for',exact=False).last.scroll_into_view_if_needed();panel=page.locator('#drawer-war');panel.screenshot(path=str(OUT/('army-'+label+'.png')))
   observations.append(dict(drawer='war',width=width,text=panel.inner_text(),overflow=panel.evaluate('e=>e.scrollWidth>e.clientWidth')))
  browser.close()
 # The actual patrol runs to completion. Readiness checks do not issue orders.
 for _ in range(180):
  returned=next(u for u in api(world+'/units',token)['units'] if u['id']==sid)
  if returned['status']=='garrison':break
  if returned['status'] in ['disbanded','sunk']:raise RuntimeError('ship lost during natural patrol')
  time.sleep(.5)
 else:raise RuntimeError('patrol did not return within bounded test wait')
 assert not returned.get('provisions_aboard') and not any(returned.get('provisions_by_good',{}).values()),returned
 (OUT/'units-returned.json').write_text(json.dumps(returned,indent=2)+'\n');snapshot('after-return');cli('unit','list');cli('notifications')
 assert not errors and not any(o['overflow'] for o in observations),(errors,observations)
 proof=dict(health=health,natural_register_cli_join_found_mixed_meal_patrol_return=True,sql_writes=False,api_replay=False,meal_tick=food['consumed_tick'],meal=food['consumed'],ship=sid,loaded_goods=loaded['provisions_by_good'],return_status=returned['status'],page_errors=errors,observations=observations)
 (OUT/'proof.json').write_text(json.dumps(proof,indent=2)+'\n');(OUT/'cli.json').write_text(json.dumps(calls,indent=2)+'\n');print(json.dumps(proof),flush=True)
finally:
 (OUT/'cli.json').write_text(json.dumps(calls,indent=2)+'\n')
 if watch is not None and watch.poll() is None:watch.terminate();watch.wait(timeout=10)
 if proc is not None:proc.terminate();proc.wait(timeout=15)
 for name in reversed(containers):subprocess.run(['docker','rm','-f',name],capture_output=True)
 (OUT/'private-player.json').unlink(missing_ok=True)
