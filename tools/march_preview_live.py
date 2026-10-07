#!/usr/bin/env python3
"""Register, join, found, preview in web+CLI, then march on a disposable world.
Usage: python3 tools/march_preview_live.py OUT EXPECTED_COMMIT
OUT must contain freshly built temenos and keryx binaries. Own Docker containers
and dynamic local ports only; no inherited game configuration or SQL fixtures.
"""
import json, os, secrets, socket, subprocess, sys, time, urllib.request, urllib.parse
from pathlib import Path
from playwright.sync_api import sync_playwright, expect
root=Path(__file__).resolve().parents[1]
out=Path(sys.argv[1]);out.mkdir(parents=True,exist_ok=True)
expected=sys.argv[2]
prefix='megaron-march-preview-'+secrets.token_hex(4)
containers=[]
proc=None

def command(*args):return subprocess.check_output(args,text=True).strip()
def port():
 with socket.socket() as s:s.bind(('127.0.0.1',0));return s.getsockname()[1]
def launch(service,image,args):
 name=prefix+'-'+service
 command('docker','run','-d','--rm','--name',name,'-p','127.0.0.1::'+('5432' if service=='pg' else '6379'),*args,image)
 containers.append(name)
 binding=json.loads(command('docker','inspect',name))[0]['NetworkSettings']['Ports']
 return int(next(iter(binding.values()))[0]['HostPort'])
try:
 pg=launch('pg','postgres:16-alpine',['-e','POSTGRES_PASSWORD=preview-pw','-e','POSTGRES_DB=preview'])
 redis=launch('redis','redis:7-alpine',[])
 for _ in range(60):
  if subprocess.run(['docker','exec',containers[0],'pg_isready','-U','postgres','-q'],capture_output=True).returncode==0:break
  time.sleep(.25)
 gameport=port();base=f'http://127.0.0.1:{gameport}'
 env={'HOME':os.environ['HOME'],'PATH':os.environ['PATH'],'DATABASE_URL':f'postgres://postgres:preview-pw@127.0.0.1:{pg}/preview?sslmode=disable','REDIS_URL':f'127.0.0.1:{redis}','JWT_SECRET':secrets.token_urlsafe(40),'PORT':str(gameport),'TICK_SECONDS':'3600','MAP_WIDTH':'30','MAP_HEIGHT':'20','WORLD_NAME':'March preview proof','POLEIA_WORLD_START_WANAXES':'1','STATIC_DIR':str(root/'web/static'),'TEMPLATE_DIR':str(root/'web/templates'),'CHRONICLE_DIR':str(out/'chronicles'),'REPORTS_DIR':str(out/'reports')}
 assert not (root/'server/.env').exists()
 def api(path,method='GET',data=None,token=None):
  request=urllib.request.Request(base+path,method=method,data=None if data is None else json.dumps(data).encode(),headers={'Content-Type':'application/json',**({'Authorization':'Bearer '+token} if token else {})})
  with urllib.request.urlopen(request,timeout=20) as response:return json.load(response)
 with (out/'server.log').open('w') as log:
  proc=subprocess.Popen([str(out/'temenos')],cwd=root/'server',env=env,stdout=log,stderr=log)
  for _ in range(120):
   try:health=api('/healthz');break
   except Exception:
    if proc.poll() is not None:raise RuntimeError('server exited; see server.log')
    time.sleep(.5)
  else:raise RuntimeError('server timeout')
  assert health['commit']==expected and health['migration']==158,health
  worlds=api('/api/v1/worlds');world=(worlds if isinstance(worlds,list) else worlds['worlds'])[0]['id']
  token=api('/api/v1/auth/register','POST',{'username':'marchproof'+secrets.token_hex(4),'password':secrets.token_urlsafe(32)})['access_token']
  worldpath='/api/v1/worlds/'+world
  api(worldpath+'/join','POST',{},token)
  founded=api(worldpath+'/founding/settle','POST',{'name':'Forecast'},token)
  assert founded.get('settlement_id')
  units=api(worldpath+'/units',token=token)['units']
  tilemap=api(worldpath+'/map',token=token)
  tiles=tilemap if isinstance(tilemap,list) else tilemap['tiles']
  chosen=None
  for unit in units:
   if not unit.get('deployable') or unit['status']!='garrison' or unit['category']!='land':continue
   for tile in tiles:
    if tile.get('tier') not in ('live','remembered'):continue
    q,r=tile['q'],tile['r']
    query=urllib.parse.urlencode({'target_q':q,'target_r':r})
    try:forecast=api(worldpath+'/units/'+unit['id']+'/march-preview?'+query,token=token)
    except Exception:continue
    if forecast.get('available'):
     chosen=(unit,q,r,forecast);break
   if chosen:break
  assert chosen, {'units':units,'error':'no valid forecast target'}
  unit,q,r,forecast=chosen
  cfg=out/'private-config.json'
  cfg.write_text(json.dumps({'server':base,'token':token,'world_id':world}));cfg.chmod(0o600)
  cli_env={'HOME':os.environ['HOME'],'PATH':os.environ['PATH'],'POLEIA_CONFIG':str(cfg)}
  cli=json.loads(subprocess.check_output([str(out/'keryx'),'march','--unit',unit['id'],'--target',f'{q},{r}','--preview','--json'],env=cli_env,text=True))
  assert cli['arrival_tick']==forecast['arrival_tick'] and cli['duration_ticks']==forecast['duration_ticks']
  assert api(worldpath+'/units',token=token)['units']==units,'preview mutated units'
  errors=[]
  with sync_playwright() as pw:
   browser=pw.chromium.launch();page=browser.new_page(viewport={'width':1280,'height':900})
   page.on('pageerror',lambda e:errors.append(str(e)))
   page.goto(base+'/',wait_until='domcontentloaded')
   page.evaluate('t=>localStorage.setItem("poleia_token",t)',token)
   page.context.add_cookies([{'name':'poleia_token','value':token,'url':base}])
   page.goto(base+'/play',wait_until='networkidle')
   page.wait_for_function('window.openDrawer !== undefined')
   page.evaluate('''async ({q,r})=>{
     window.previewMenu=await import('/static/js/megaron/ui/marchctx.js');
     await previewMenu.openMarchCtx({q,r,known:true,isSea:false,isSettlement:true,name:'Known destination'},120,120);
   }''',{'q':q,'r':r})
   index=page.evaluate('''async id=>(await import('/static/js/megaron/state.js')).State.marchCtxGroups.findIndex(g=>g.ids.includes(id))''',unit['id'])
   assert index>=0
   page.locator('#mg-'+str(index)).fill('1')
   expect(page.locator('#mctx-eta')).to_contain_text('Estimated arrival')
   expect(page.locator('#mctx-eta')).to_contain_text(str(forecast['duration_ticks'])+' game days')
   page.locator('#march-ctx').screenshot(path=str(out/'live-desktop.png'))
   page.set_viewport_size({'width':390,'height':844})
   page.evaluate('previewMenu.closeMarchCtx()')
   page.evaluate('''async ({q,r})=>previewMenu.openMarchCtx({q,r,known:true,isSea:false,isSettlement:true,name:'Known destination'},100,100)''',{'q':q,'r':r})
   page.locator('#mg-'+str(index)).fill('1')
   expect(page.locator('#mctx-eta')).to_contain_text('Estimated arrival')
   page.locator('#march-ctx').screenshot(path=str(out/'live-mobile.png'))
   assert page.locator('#march-ctx').evaluate('(el)=>el.scrollWidth<=el.clientWidth')
   # The actual menu sends the march after showing the estimate.
   page.locator('#mctx-send').click()
   expect(page.locator('#mctx-eta')).to_contain_text('Marching')
   browser.close()
  after=next(u for u in api(worldpath+'/units',token=token)['units'] if u['id']==unit['id'])
  assert after['status']=='marching',after
  assert after.get('arrival_tick')==forecast['arrival_tick'],after
  assert not errors,errors
  proof={'health':health,'registered_joined_founded':True,'actual_cli_preview':cli,'actual_api_preview':forecast,'browser_sent_march':True,'actual_arrival_tick':after['arrival_tick'],'browser_errors':errors,'scope':'real isolated server, PostgreSQL16, Redis7, player APIs, CLI and web; no SQL fixture edits'}
  (out/'proof.json').write_text(json.dumps(proof,indent=2)+'\n')
  print(json.dumps(proof))
finally:
 if proc is not None:
  proc.terminate();proc.wait(timeout=20)
 for name in containers:subprocess.run(['docker','rm','-f',name],capture_output=True)
 if (out/'private-config.json').exists():(out/'private-config.json').unlink()
