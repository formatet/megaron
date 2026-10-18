#!/usr/bin/env python3
"""Natural slice12 CLI/Firefox proof; no SQL writes, live accounts or API overrides."""
from pathlib import Path
import json, os, secrets, socket, subprocess, sys, time, urllib.request
from playwright.sync_api import sync_playwright
from concurrent.futures import ThreadPoolExecutor
import urllib.error
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
 assert health['commit']==COMMIT and health['migration']==183,health
 worlds=api('/api/v1/worlds');world=(worlds if isinstance(worlds,list) else worlds['worlds'])[0]['id'];wp='/api/v1/worlds/'+world
 candidates={};attempts=[]
 for attempt in range(35):
  token=api('/api/v1/auth/register',data={'username':'surface-'+secrets.token_hex(6),'password':secrets.token_urlsafe(32)})['access_token']
  joined=api(wp+'/join',token,{});q,r=joined['tile']['Q'],joined['tile']['R']
  site=api(wp+f"/colonize-preview?q={q}&r={r}&pop=1000&seed=1200&starter_farm=1",token);g=site['grain']
  kind='inland' if g['starter_farm_included'] and g['fish_per_tick']==0 and g['food_net_per_tick']>=0 else 'coast' if g['fish_per_tick']>0 and g['food_net_per_tick']>=0 else None
  attempts.append({'kind':kind,'q':q,'r':r,'grain':g['base_per_tick'],'fish':g['fish_per_tick'],'net':g['food_net_per_tick']})
  if kind and kind not in candidates:candidates[kind]=(token,joined,site,q,r,attempt)
  if len(candidates)==2:break
 else:raise AssertionError(attempts)
 subprocess.run(['go','build','-o',str(OUT/'keryx'),'./cmd/keryx'],cwd=ROOT/'server',check=True)
 errors=[];observed={}
 for kind,(token,joined,site,q,r,attempt) in candidates.items():
  config=OUT/('private-'+kind+'.json');config.write_text(json.dumps({'server':base,'token':token,'opaque':'preserve'}));config.chmod(0o600)
  envcli={k:os.environ[k] for k in ('HOME','PATH')};envcli['POLEIA_CONFIG']=str(config)
  def cli(label,*args):
   result=subprocess.run([str(OUT/'keryx'),*args],env=envcli,input='',text=True,capture_output=True)
   text=result.stdout+result.stderr;(OUT/(kind+'-'+label+'.txt')).write_text(text);return result,text
  result,status=cli('auto-world','status');assert result.returncode==0,status
  saved=json.loads(config.read_text());assert saved['world_id']==world and saved['opaque']=='preserve'
  result,text=cli('forecast','founding','settle','--name',kind+'-proof')
  assert result.returncode and 'Nothing sent' in text and 'no farmable' not in text and '-0.0' not in text,text
  if kind=='inland' and site['grain']['food_net_per_tick']<0:assert 'already included' in text,text
  result,host=cli('host-colonize','unit','march','--unit',joined['host_unit_id'],'--target',f'{q},{r}','--intent','colonize','--preview')
  assert result.returncode and 'founding settle' in host and 'colonists become' not in host,host
  notices=api(wp+'/notifications',token);opening=next(n['body'] for n in notices['notifications'] if n['kind']=='OpeningDispatch')
  openingtext=' '.join(opening['paragraphs']);assert 'Place citizens' in openingtext,openingtext
  if attempt>0:assert 'Days begin after' not in openingtext,openingtext
  (OUT/(kind+'-forecast.json')).write_text(json.dumps(site,indent=2));(OUT/(kind+'-dispatch.json')).write_text(json.dumps(opening,indent=2))
  with sync_playwright() as pw:
   browser=pw.firefox.launch();page=browser.new_page(viewport={'width':1280,'height':900},device_scale_factor=1)
   page.on('pageerror',lambda e:errors.append(str(e)))
   page.goto(base+'/',wait_until='domcontentloaded');page.evaluate('t=>localStorage.setItem("poleia_token",t)',token)
   page.context.add_cookies([{'name':'poleia_token','value':token,'url':base}]);page.goto(base+'/play',wait_until='networkidle')
   page.wait_for_function('window.openDrawer !== undefined')
   point=page.evaluate("""async ({q,r})=>{const {State}=await import('/static/js/megaron/state.js');const {hexPx,SCALE}=await import('/static/js/megaron/render/map.js');const p=hexPx(q,r),rect=document.getElementById('hex-canvas').getBoundingClientRect();State.camera.x=rect.width/2-p.x*State.camera.zoom*SCALE;State.camera.y=rect.height/2-p.y*State.camera.zoom*SCALE;State.dirty=true;return {x:rect.left+rect.width/2,y:rect.top+rect.height/2};}""",{'q':q,'r':r})
   page.wait_for_timeout(200);page.mouse.click(point['x'],point['y']);page.wait_for_selector('#ip-host-details');page.locator('#ip-host-details summary').click()
   page.wait_for_function("document.getElementById('ip-host-details')?.textContent.includes('consumption')")
   screen=page.locator('#ip-host-details').inner_text();assert 'no farmland' not in screen and '-0.0' not in screen,screen
   (OUT/(kind+'-forecast-web.txt')).write_text(screen);page.screenshot(path=str(OUT/(kind+'-forecast-firefox.png')))
   page.set_viewport_size({'width':390,'height':844});page.screenshot(path=str(OUT/(kind+'-forecast-mobile.png')));browser.close()
  result,foundedtext=cli('founded','founding','settle','--name',kind+'-proof','--yes');assert result.returncode==0 and '[y/N]' not in foundedtext and 'Found the metropolis here?' not in foundedtext,foundedtext
  closed=api(wp+'/notifications',token)
  payload=next(n['body'] for n in closed['notifications'] if n['kind']=='MetropolisFounded')
  assert 'food_net_per_tick' in payload,payload
  if payload['food_net_per_tick']>=-1e-9:assert 'grain_ticks' not in payload and 'grain_days' not in payload,payload
  (OUT/(kind+'-founded-notice.json')).write_text(json.dumps(payload,indent=2))
  _,notice=cli('closed-notification','notifications')
  assert 'food production covers consumption' in notice and '675539944105574400' not in notice,notice
  saved=json.loads(config.read_text());province=api(wp+'/provinces/'+saved['province_id'],token)['settlement']
  if kind=='inland':assert any(b.get('type')=='farm' and b.get('level')==1 for b in province['buildings']),province['buildings']
  _,city=cli('city','city');assert '#' in city and '(' in city and 'Automatic food:' in city and 'could move' not in city and '★' not in city,city
  pid=saved['province_id']
  for label,args in [('city-flag',['city','--province',pid]),('placements-flag',['placements','--province',pid]),('status-pos',['status',pid]),('status-flag',['status','--province',pid]),('build-pos',['build',pid,'--queue'])]:
   run,txt=cli(label,*args);assert run.returncode==0,txt
  assert 'fixed limit; population and buildings do not add slots' in txt,txt
  options=api(wp+'/provinces/'+pid+'/placement-options',token)
  if kind=='inland':
   run,txt=cli('hex-staff','staff','farm','+1');assert run.returncode and 'hex building' in txt and 'keryx place grain' in txt,txt
  # A real player's outgoing exploration order, not seeded DB state.
  run,unitsraw=cli('units-before-march','unit','list','--json');assert run.returncode==0,unitsraw
  units=json.loads(unitsraw)['units']
  soldier=next(u for u in units if u['type']=='spearman' and u['status']=='garrison')
  march_attempts=[]
  known=api(wp+'/map',token)
  def distance(t):return max(abs(t['q']-q),abs(t['r']-r),abs(t['q']-q+t['r']-r))
  targets=sorted([t for t in known if t['tier'] in ('live','remembered') and t['terrain'] in ('plains','hills','river_valley','river_delta','forest_olive_grove','forest_cedar','scrub_maquis','semi_desert') and distance(t)>=3],key=distance,reverse=True)
  assert targets,'no known land to explore from'
  for target in targets[:12]:
   tq,tr=target['q'],target['r']
   run,txt=cli('explore-'+str(len(march_attempts)),'unit','march','--unit',soldier['id'],'--q',str(tq),'--r',str(tr),'--intent','explore','--ticks','30')
   march_attempts.append({'target':[tq,tr],'returncode':run.returncode,'text':txt})
   if run.returncode==0:break
  else:raise AssertionError(march_attempts)
  run,txt=cli('units-marching','unit','list','--json');assert run.returncode==0,txt
  assert any(u['id']==soldier['id'] and u['status']=='marching' for u in json.loads(txt)['units']),txt
  (OUT/(kind+'-march-evidence.json')).write_text(json.dumps({'attempts':march_attempts,'marching_before_placements':True},indent=2))
  # Natural concurrency, no SQL barrier/fixture or response overrides here.
  target=next(h for h in options['hexes'] if not h.get('held_by') and any(g['good_key']=='stone' and g.get('placed',0)<g.get('cap',0) for g in h['goods']))
  def batch(method,suffix,data=None):
   request=urllib.request.Request(base+wp+'/provinces/'+pid+'/placements'+suffix,method=method,data=None if data is None else json.dumps(data).encode(),headers={'Content-Type':'application/json','Authorization':'Bearer '+token})
   try:
    with urllib.request.urlopen(request,timeout=15) as response:return response.status,json.load(response)
   except urllib.error.HTTPError as e:return e.code,json.load(e)
  with ThreadPoolExecutor(max_workers=8) as workers:created=list(workers.map(lambda _:batch('POST','',{'target_kind':'hex','hex_ordinal':target['hex_ordinal'],'good_key':'stone'}),range(8)))
  assert all(code in (201,409) for code,_ in created),created
  ordinals=[v['gubbe_ordinal'] for code,v in created if code==201];assert ordinals,created
  # Stone has one natural place here; remove each actually created worker.
  deleted=[]
  for ordinal in ordinals:
   with ThreadPoolExecutor(max_workers=8) as workers:responses=list(workers.map(lambda _:batch('DELETE','/'+str(ordinal)),range(8)))
   assert sum(code==200 for code,_ in responses)==1 and all(code in (200,404) for code,_ in responses),responses
   deleted+=responses
  (OUT/(kind+'-parallel.json')).write_text(json.dumps({'post':created,'delete':deleted},indent=2))
  options=api(wp+'/provinces/'+pid+'/placement-options',token)
  assert options['pool_size']>=0,options
  occupied=next(((h,g) for h in options['hexes'] if not h.get('held_by') for g in h['goods'] if g['good_key'] in ('grain','fish') and g.get('placed_ordinals')),None)
  assert occupied,options
  h,g=occupied
  wrong=next(x for x in options['hexes'] if x['hex_ordinal']!=h['hex_ordinal'] and not any(z['good_key']==g['good_key'] and z.get('placed_ordinals') for z in x['goods']))
  run,txt=cli('empty-remove','place',pid,g['good_key'],str(wrong['hex_ordinal']),'-1');assert run.returncode==0 and ('#'+str(h['hex_ordinal'])+' '+g['good_key']) in txt,txt
  run,txt=cli('place-remove','place','--province',pid,'--hex','#'+str(h['hex_ordinal']),g['good_key'],'-1');assert run.returncode==0,txt
  run,txt=cli('place-restore','place',pid,g['good_key'],str(h['hex_ordinal']),'+1');assert run.returncode==0,txt
  province=api(wp+'/provinces/'+pid,token)['settlement']
  assert abs(province['food_net_per_tick_after_upkeep']-(province['sitos']['food_net_per_tick']-province['army_upkeep']['grain']))<1e-9,province
  run,txt=cli('army-status','status');assert run.returncode==0 and 'after army' in txt and 'Net      net' not in txt and '1 citizen = 100 people' in txt,txt

  _,goods=cli('goods','goods');assert 'Per day' in goods and 'Rate/t' not in goods,goods
  for _ in range(100):
   province=api(wp+'/provinces/'+saved['province_id'],token)['settlement']
   if province['sitos']['granary_per_good'].get('grain',0)>0:break
   time.sleep(.2)
  else:raise AssertionError({'no_natural_granary':province['sitos']})
  (OUT/(kind+'-grown-city.json')).write_text(json.dumps(province,indent=2))
  a=province['automatic_food'];assert a['remainder_people']==province['population']%100,a
  assert a['remainder_grain_per_tick']>=0 and a['remainder_fish_per_tick']>=0,a
  _,status=cli('status','status');assert 'Food     grain' in status and 'grain in Sitos' in status and 'Automatic food:' in status and 'could move' not in status,status
  with sync_playwright() as pw:
   browser=pw.firefox.launch();page=browser.new_page(viewport={'width':1280,'height':900},device_scale_factor=1);page.on('pageerror',lambda e:errors.append(str(e)))
   page.goto(base+'/',wait_until='domcontentloaded');page.evaluate('t=>localStorage.setItem("poleia_token",t)',token);page.context.add_cookies([{'name':'poleia_token','value':token,'url':base}]);page.goto(base+'/play',wait_until='networkidle');page.wait_for_function('window.openDrawer !== undefined')
   page.locator('#trig-economy').click();page.wait_for_selector('#ectab-goods .goods-mini');page.wait_for_function("document.getElementById('ectab-goods').textContent.includes('in Sitos')");page.wait_for_timeout(250)
   screen=page.locator('#ectab-goods').inner_text();assert 'Food after army' in screen and 'in Sitos' in screen,screen
   (OUT/(kind+'-goods-web.txt')).write_text(screen);page.screenshot(path=str(OUT/(kind+'-goods-firefox.png')))
   page.locator('#ectab-goods tr[onclick^="openCitySettlement"]').first.click()
   page.wait_for_selector('#city-more summary');page.locator('#city-more summary').click();page.wait_for_function("document.getElementById('city-reserve-sec')?.textContent.includes('after army')")
   details=page.locator('#city-reserve-sec').inner_text();assert 'Army food' in details and 'before army' in details and 'Automatic food:' in details,details
   (OUT/(kind+'-city-food-web.txt')).write_text(details);page.locator('#city-reserve-sec').scroll_into_view_if_needed();page.screenshot(path=str(OUT/(kind+'-city-food-firefox.png')))
   page.set_viewport_size({'width':390,'height':844});page.locator('#city-reserve-sec').scroll_into_view_if_needed();page.screenshot(path=str(OUT/(kind+'-city-food-mobile.png')))
   browser.close()
  observed[kind]={'site':{'q':q,'r':r},'province_id':saved['province_id'],'food_net':province['sitos']['food_net_per_tick'],'granary':province['sitos']['granary_per_good']}
 assert not errors,errors
 (OUT/'evidence.json').write_text(json.dumps({'health':health,'attempts':attempts,'observed':observed,'page_errors':errors},indent=2))
 print('slice12 real exploration -> parallel placement and CLI, Firefox1280/390: GREEN',flush=True)
finally:
 if proc:proc.terminate();proc.wait(timeout=15)
 for name in reversed(containers):subprocess.run(['docker','rm','-fv',name],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
