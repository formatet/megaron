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
 assert health['commit']==COMMIT and health['migration']==182,health
 worlds=api('/api/v1/worlds');world=(worlds if isinstance(worlds,list) else worlds['worlds'])[0]['id'];wp='/api/v1/worlds/'+world
 candidates={};attempts=[]
 for attempt in range(35):
  token=api('/api/v1/auth/register',data={'username':'surface-'+secrets.token_hex(6),'password':secrets.token_urlsafe(32)})['access_token']
  joined=api(wp+'/join',token,{});q,r=joined['tile']['Q'],joined['tile']['R']
  site=api(wp+f"/colonize-preview?q={q}&r={r}&pop=1000&seed=1200&starter_farm=1",token);g=site['grain']
  kind='inland' if g['starter_farm_included'] and g['food_net_per_tick']>=0 else 'coast' if g['fish_per_tick']>0 and g['food_net_per_tick']>=0 else None
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
  result,foundedtext=cli('founded','founding','settle','--name',kind+'-proof','--yes');assert result.returncode==0,foundedtext
  closed=api(wp+'/notifications',token)
  payload=next(n['body'] for n in closed['notifications'] if n['kind']=='MetropolisFounded')
  assert 'food_net_per_tick' in payload,payload
  if payload['food_net_per_tick']>=-1e-9:assert 'grain_ticks' not in payload and 'grain_days' not in payload,payload
  (OUT/(kind+'-founded-notice.json')).write_text(json.dumps(payload,indent=2))
  _,notice=cli('closed-notification','notifications')
  assert 'food production covers consumption' in notice and '675539944105574400' not in notice,notice
  saved=json.loads(config.read_text());province=api(wp+'/provinces/'+saved['province_id'],token)['settlement']
  if kind=='inland':assert any(b.get('type')=='farm' and b.get('level')==1 for b in province['buildings']),province['buildings']
  _,city=cli('city','city');assert '#' in city and '(' in city,city
  _,goods=cli('goods','goods');assert 'Per day' in goods and 'Rate/t' not in goods,goods
  for _ in range(100):
   province=api(wp+'/provinces/'+saved['province_id'],token)['settlement']
   if province['sitos']['granary_per_good'].get('grain',0)>0:break
   time.sleep(.2)
  else:raise AssertionError({'no_natural_granary':province['sitos']})
  _,status=cli('status','status');assert 'Food     grain' in status and 'grain in Sitos' in status,status
  with sync_playwright() as pw:
   browser=pw.firefox.launch();page=browser.new_page(viewport={'width':1280,'height':900},device_scale_factor=1);page.on('pageerror',lambda e:errors.append(str(e)))
   page.goto(base+'/',wait_until='domcontentloaded');page.evaluate('t=>localStorage.setItem("poleia_token",t)',token);page.context.add_cookies([{'name':'poleia_token','value':token,'url':base}]);page.goto(base+'/play',wait_until='networkidle');page.wait_for_function('window.openDrawer !== undefined')
   page.locator('#trig-economy').click();page.wait_for_selector('#ectab-goods .goods-mini');page.wait_for_function("document.getElementById('ectab-goods').textContent.includes('in Sitos')");page.wait_for_timeout(250)
   screen=page.locator('#ectab-goods').inner_text();assert 'Food net' in screen and 'in Sitos' in screen,screen
   (OUT/(kind+'-goods-web.txt')).write_text(screen);page.screenshot(path=str(OUT/(kind+'-goods-firefox.png')));browser.close()
  observed[kind]={'site':{'q':q,'r':r},'province_id':saved['province_id'],'food_net':province['sitos']['food_net_per_tick'],'granary':province['sitos']['granary_per_good']}
 assert not errors,errors
 (OUT/'evidence.json').write_text(json.dumps({'health':health,'attempts':attempts,'observed':observed,'page_errors':errors},indent=2))
 print('natural inland/coastal forecast, world selection, Host guidance, founding, CLI city/goods/status and web Sitos: GREEN',flush=True)
finally:
 if proc:proc.terminate();proc.wait(timeout=15)
 for name in reversed(containers):subprocess.run(['docker','rm','-fv',name],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
