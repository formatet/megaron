#!/usr/bin/env python3
"""Slice 20: ordinary CLI/API/Firefox flow against isolated local services."""
from pathlib import Path
import json, os, secrets, socket, subprocess, sys, time, urllib.request
from playwright.sync_api import sync_playwright
ROOT=Path(__file__).resolve().parents[3]
OUT=Path(sys.argv[1]).resolve();OUT.mkdir(parents=True,exist_ok=True)
COMMIT=subprocess.check_output(['git','-C',str(ROOT),'rev-parse','HEAD'],text=True).strip()
DIRTY=bool(subprocess.check_output(['git','-C',str(ROOT),'diff','HEAD','--','server','web','tools'],text=True).strip())
containers=[];proc=None;log=None

def command(*args):return subprocess.check_output(args,text=True).strip()
def service(label,image,args,port):
 name='megaron-loyalty-proof-'+secrets.token_hex(5)+'-'+label
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
 env.update(DATABASE_URL=f'postgres://postgres:proof@127.0.0.1:{pg}/proof?sslmode=disable',REDIS_URL='127.0.0.1:'+redis,JWT_SECRET=secrets.token_urlsafe(32),PORT=str(port),TICK_SECONDS='60',MAP_WIDTH='56',MAP_HEIGHT='40',WORLD_NAME='Loyalty removal proof',POLEIA_WORLD_START_WANAXES='1',STATIC_DIR=str(ROOT/'web/static'),TEMPLATE_DIR=str(ROOT/'web/templates'),CHRONICLE_DIR=str(OUT/'chronicles'),REPORTS_DIR=str(OUT/'reports'))
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
 assert health['commit']==COMMIT and health['migration']==185,health
 username='surface-'+secrets.token_hex(6);password=secrets.token_urlsafe(32)
 api('/api/v1/auth/register',data={'username':username,'password':password})
 config=OUT/'private-config.json'
 envcli={k:os.environ[k] for k in ('HOME','PATH')};envcli.update(POLEIA_CONFIG=str(config),POLEIA_PASSWORD=password)
 def cli(label,*args):
  result=subprocess.run([str(OUT/'keryx'),*args],env=envcli,input='',text=True,capture_output=True)
  text=result.stdout+result.stderr;(OUT/(label+'.txt')).write_text(text);assert result.returncode==0,text
  return text
 cli('login','login','--server',base,'--username',username)
 cli('join','join');cli('found','founding','settle','--name','Proofpolis','--yes')
 saved=json.loads(config.read_text());token=saved['token'];wp='/api/v1/worlds/'+saved['world_id']
 for label,args in [('status',['status']),('city',['city']),('ticklog',['ticklog'])]:
  text=cli(label,*args);assert 'loyalty' not in text.lower() and 'Gift from capital' not in text,text
 default=api(wp+'/retreat-default',token);assert default['when_losing'] and not default['hold_to_last_man'],default
 for label,flag,hold in [('retreat-hold','--hold-to-last-man',True),('retreat-reset','--when-losing',False)]:
  text=cli(label,'retreat-default',flag);readback=api(wp+'/retreat-default',token);assert readback['hold_to_last_man']==hold and readback['when_losing']==(not hold),readback
 province=api(wp+'/provinces/'+saved['province_id'],token)
 settlements=api(wp+'/settlements',token)
 def no_loyalty(value):
  if isinstance(value,dict):
   assert not any(k.startswith('loyalty') for k in value),value
   for v in value.values():no_loyalty(v)
  elif isinstance(value,list):
   for v in value:no_loyalty(v)
 no_loyalty(province);no_loyalty(settlements)
 (OUT/'province.json').write_text(json.dumps(province,indent=2));(OUT/'settlements.json').write_text(json.dumps(settlements,indent=2))
 errors=[];screens={}
 with sync_playwright() as pw:
  browser=pw.firefox.launch();page=browser.new_page(viewport={'width':1280,'height':900},device_scale_factor=1)
  page.on('pageerror',lambda e:errors.append(str(e)))
  page.goto(base+'/',wait_until='domcontentloaded');page.evaluate('t=>localStorage.setItem("poleia_token",t)',token);page.context.add_cookies([{'name':'poleia_token','value':token,'url':base}]);page.goto(base+'/play',wait_until='networkidle');page.wait_for_function('window.openDrawer !== undefined')
  for size in [{'width':1280,'height':900},{'width':390,'height':844}]:
   page.set_viewport_size(size);suffix=str(size['width'])
   page.evaluate("document.querySelectorAll('.drawer.open').forEach(e=>closeDrawer(e.id.slice(7)));openDrawer('city')");page.wait_for_selector('#city-more summary');page.locator('#city-more').evaluate('(e)=>e.open=true')
   page.wait_for_function("document.getElementById('city-food-details-sec')?.textContent.includes('Army food')")
   city=page.locator('#city-more').inner_text();assert 'loyalty' not in city.lower() and 'Gift from capital' not in city,city
   (OUT/('city-'+suffix+'.txt')).write_text(city);page.locator('#city-more').scroll_into_view_if_needed();page.wait_for_function("document.querySelectorAll('.drawer.open').length===1 && document.getElementById('drawer-city').classList.contains('open')");page.evaluate("async()=>await Promise.all([...document.getElementById('drawer-city').getAnimations()].map(a=>a.finished.catch(()=>{})))");page.screenshot(path=str(OUT/('city-'+suffix+'.png')))
   page.evaluate("closeDrawer('city');openDrawer('war')");page.wait_for_selector('#war-retreat-default');page.wait_for_function("document.getElementById('war-retreat-default')?.value==='when_losing'")
   options=page.locator('#war-retreat-default option').all_text_contents();assert options==['Retreat when it goes badly','Hold to the last man'],options
   page.locator('#war-retreat-default').scroll_into_view_if_needed();page.screenshot(path=str(OUT/('war-'+suffix+'.png')));screens[suffix]={'city':city,'retreat_options':options}
  browser.close()
 assert not errors,errors
 (OUT/'evidence.json').write_text(json.dumps({'commit':COMMIT,'dirty':DIRTY,'dirty_scope':'tracked server/web/tools versus HEAD','health':health,'retreat_default':api(wp+'/retreat-default',token),'screens':screens,'page_errors':errors},indent=2))
 print('CLI/API and Firefox 1280/390: GREEN',flush=True)
finally:
 if proc:proc.terminate();proc.wait(timeout=15)
 if log:log.close()
 for name in reversed(containers):subprocess.run(['docker','rm','-fv',name],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
 for name in ('private-config.json','temenos','keryx'):(OUT/name).unlink(missing_ok=True)
