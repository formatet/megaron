#!/usr/bin/env python3
"""Natural founder CLI forecast, catalogue, prayers, notifications and real web menu."""
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
  token=api('/api/v1/auth/register',data={'username':'river-'+secrets.token_hex(6),'password':secrets.token_urlsafe(32)})['access_token']
  joined=api(wp+'/join',token,{})
  raw=api(wp+'/map',token);tiles=raw if isinstance(raw,list) else raw['tiles']
  spawn=joined['tile'];home=next((t for t in tiles if t['q']==spawn['Q'] and t['r']==spawn['R']),{})
  rivers=[t for t in tiles if t.get('terrain')=='river' and t.get('tier') in ('live','remembered')]
  attempts.append({'coastal':home.get('coastal'),'visible_rivers':len(rivers)})
  if home.get('coastal') and rivers:
   site=api(wp+f"/colonize-preview?q={spawn['Q']}&r={spawn['R']}&pop=1000&seed=0",token)
   if site.get('timber_warning'):break
 else:raise AssertionError({'no_coastal_river_spawn':attempts})
 subprocess.run(['go','build','-o',str(OUT/'keryx'),'./cmd/keryx'],cwd=ROOT/'server',check=True)
 config=OUT/'private-config.json';config.write_text(json.dumps({'server':base,'token':token,'world_id':world}));config.chmod(0o600)
 cli_env={k:os.environ[k] for k in ('HOME','PATH')};cli_env['POLEIA_CONFIG']=str(config)
 def cli(label,*args):
  result=subprocess.run([str(OUT/'keryx'),*args],env=cli_env,input='',text=True,capture_output=True)
  text=result.stdout+result.stderr;(OUT/(label+'.txt')).write_text(text)
  return result,text
 result,text=cli('founder-forecast','founding','settle','--name','Potamos')
 assert 'consumption ~5.0' in text and 'No timber' in text and 'the city starves' not in text,text
 zero_forecast=api(wp+f"/colonize-preview?q={spawn['Q']}&r={spawn['R']}&pop=1000&seed=0",token)
 assert zero_forecast['grain']['consumption_per_tick']==5 and zero_forecast['grain']['loyalty_warning'],zero_forecast
 assert zero_forecast['catchment'][0]['q']==spawn['Q'] and zero_forecast['catchment'][0]['r']==spawn['R'],zero_forecast
 assert zero_forecast['harbour_note'].startswith('Coastal:'),zero_forecast
 assert 'Poseidon' in text and 'Demeter' not in text,text
 (OUT/'zero-stock-forecast.json').write_text(json.dumps(zero_forecast,indent=2))
 with sync_playwright() as hostpw:
  hostbrowser=hostpw.firefox.launch();hostpage=hostbrowser.new_page(viewport={'width':1280,'height':900})
  hostpage.goto(base+'/',wait_until='domcontentloaded');hostpage.evaluate('t=>localStorage.setItem("poleia_token",t)',token)
  hostpage.context.add_cookies([{'name':'poleia_token','value':token,'url':base}]);hostpage.goto(base+'/play',wait_until='networkidle')
  hostpage.wait_for_function('window.openDrawer !== undefined')
  hostpoint=hostpage.evaluate('''async ({q,r})=>{const {State}=await import('/static/js/megaron/state.js');const {hexPx,SCALE}=await import('/static/js/megaron/render/map.js');const p=hexPx(q,r),rect=document.getElementById('hex-canvas').getBoundingClientRect();State.camera.x=rect.width/2-p.x*State.camera.zoom*SCALE;State.camera.y=rect.height/2-p.y*State.camera.zoom*SCALE;State.dirty=true;return {x:rect.left+rect.width/2,y:rect.top+rect.height/2};}''',{'q':spawn['Q'],'r':spawn['R']})
  hostpage.wait_for_timeout(200);hostpage.mouse.click(hostpoint['x'],hostpoint['y'])
  hostpage.wait_for_function("document.getElementById('ip-found-summary')?.textContent.includes('No timber')")
  assert not hostpage.locator('#ip-host-details').evaluate('el=>el.open')
  hostpage.screenshot(path=str(OUT/'founder-warning-firefox.png'))
  hostpage.set_viewport_size({'width':390,'height':844});hostpage.screenshot(path=str(OUT/'founder-warning-firefox-mobile.png'))
  assert hostpage.locator('#ip-settle-btn').is_visible()
  hostbrowser.close()
 founded=api(wp+'/founding/settle',token,{'name':'Potamos'})
 for _ in range(60):
  response=api(wp+'/units',token)
  ship=next((u for u in response['units'] if u['category']=='naval' and u['status']=='garrison' and u.get('deployable')),None)
  if ship:break
  time.sleep(.25)
 assert ship,response
 config.write_text(json.dumps({'server':base,'token':token,'world_id':world,'province_id':founded['province_id']}));config.chmod(0o600)
 _,status=cli('status','status')
 for term in ('harbour (16)','barracks (8)','foundry (8)','temple (8)'):assert term in status,status
 _,prayers=cli('prayers','rite','--list');assert 'no: no temple' in prayers and 'Offering values' in prayers and 'workday' not in prayers.lower(),prayers
 _,goods=cli('goods','goods');assert 'workday' not in goods.lower(),goods
 altar=api(wp+'/provinces/'+founded['province_id'],token)['settlement']['divine_values']
 assert altar and altar.get('bronze',0)>0,altar
 (OUT/'divine-values.json').write_text(json.dumps(altar,indent=2))
 _,notices=cli('notifications','notifications');assert 'Your metropolis is founded: Potamos' in notices and 'Poseidon grants a galley' in notices,notices
 _,helptext=cli('help','--help');assert 'POLEIA_CONFIG' in helptext,helptext
 bad,error=cli('bad-flag','--unknown-proof-flag');assert bad.returncode and 'Usage:' not in error and 'See keryx --help' in error,error
 ticks=response['expedition_rules']['default_ticks']
 for tile in rivers:
  try:
   preview=api(wp+'/units/'+ship['id']+f"/march-preview?target_q={tile['q']}&target_r={tile['r']}&intent=explore&ticks={ticks}",token)
   if 'available' in preview:
    assert preview['planned_expedition_ticks']==ticks and 'planned_home_by_tick_if_departing_now' in preview and 'arrival_tick' not in preview,preview
    print('actual preview',json.dumps(preview),flush=True)
    target=tile;break
  except Exception:pass
 else:raise AssertionError({'no_reachable_river':rivers})
 _,plan=cli('expedition-plan','unit','march','--unit',ship['id'],'--q',str(target['q']),'--r',str(target['r']),'--intent','explore','--ticks',str(ticks),'--preview')
 assert 'Planned expedition:' in plan and 'with immediate departure' in plan and 'First-leg arrival remains unknown' in plan,plan
 errors=[]
 with sync_playwright() as pw:
  browser=pw.chromium.launch();page=browser.new_page(viewport={'width':1280,'height':900})
  page.on('pageerror',lambda e:errors.append(str(e)))
  page.goto(base+'/',wait_until='domcontentloaded');page.evaluate('t=>localStorage.setItem("poleia_token",t)',token)
  page.context.add_cookies([{'name':'poleia_token','value':token,'url':base}]);page.goto(base+'/play',wait_until='networkidle')
  page.wait_for_function('window.openDrawer !== undefined')
  page.evaluate("window.openDrawer('economy')")
  page.wait_for_selector('#ectab-goods .silver-balance')
  assert 'workday' not in page.locator('#ectab-goods').inner_text().lower()
  page.screenshot(path=str(OUT/'goods.png'))
  page.evaluate("window.openDrawer('kult')")
  page.wait_for_function("document.getElementById('kult-body').textContent.includes('How the gods weigh gifts')")
  assert 'bronze' in page.locator('#kult-body').inner_text() and 'workday' not in page.locator('#kult-body').inner_text().lower()
  page.screenshot(path=str(OUT/'kult.png'))
  page.set_viewport_size({'width':390,'height':844});page.screenshot(path=str(OUT/'kult-mobile.png'))
  assert page.evaluate('document.documentElement.scrollWidth <= window.innerWidth')
  page.set_viewport_size({'width':1280,'height':900})
  page.evaluate("window.closeDrawer('kult');window.closeDrawer('economy');window.closeDrawer('city');window.closeDrawer('war');window.closeInspect()")
  point=page.evaluate('''async ({q,r})=>{const {State}=await import('/static/js/megaron/state.js');const {hexPx,SCALE}=await import('/static/js/megaron/render/map.js');const p=hexPx(q,r),rect=document.getElementById('hex-canvas').getBoundingClientRect();State.camera.x=rect.width/2-p.x*State.camera.zoom*SCALE;State.camera.y=rect.height/2-p.y*State.camera.zoom*SCALE;State.dirty=true;return {x:rect.left+rect.width/2,y:rect.top+rect.height/2};}''',{'q':target['q'],'r':target['r']})
  page.wait_for_timeout(200);page.mouse.click(point['x'],point['y'],button='right');page.wait_for_selector('#mg-0')
  groups=page.evaluate("async()=> (await import('/static/js/megaron/state.js')).State.marchCtxGroups")
  assert groups and all(g['category']=='naval' for g in groups),groups
  page.locator('#mctx-eta').wait_for(state='visible');page.wait_for_timeout(500)
  page.screenshot(path=str(OUT/'river-menu.png'))
  with page.expect_response(lambda r:'/units/'+ship['id']+'/march' in r.url and r.request.method=='POST') as sent:
   page.locator('#mctx-send').click()
  result=sent.value;body=result.request.post_data_json;answer=result.json()
  assert result.ok and body['intent']=='explore' and body['target_q']==target['q'] and body['target_r']==target['r'],(body,answer)
  movement=[]
  for _ in range(80):
   current=next(u for u in api(wp+'/units',token)['units'] if u['id']==ship['id'])
   movement.append({'status':current['status'],'q':current.get('q'),'r':current.get('r')})
   if current['status']=='marching':break
   time.sleep(.25)
  else:raise AssertionError({'receipt_without_movement':answer,'observed':movement})
  assert not errors,errors
  (OUT/'evidence.json').write_text(json.dumps({'health':health,'attempts':attempts,'target':target,'ship_id':ship['id'],'groups':groups,'preview':preview,'sent':body,'http_status':result.status,'answer':answer,'movement':movement,'page_errors':errors},indent=2))
  browser.close()
 print('natural wood warning + conditional expedition + divine values under Kult/rite + no Goods valuation unit: GREEN')
finally:
 if proc:proc.terminate();proc.wait(timeout=15)
 for name in reversed(containers):subprocess.run(['docker','rm','-fv',name],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
