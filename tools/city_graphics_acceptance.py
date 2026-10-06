#!/usr/bin/env python3
"""Real-server city graphics smoke in a disposable LOCAL database.

Requires dedicated PostgreSQL city_test at 127.0.0.1:18732 and Redis at
127.0.0.1:18579 (see proof report). OUT/temenos is a freshly built server.
Usage: python3 tools/city_graphics_acceptance.py OUT EXPECTED_BUILD_COMMIT
No inherited game configuration or production endpoint is used.
"""
import os, json, secrets, subprocess, time, urllib.request, urllib.error, hashlib, sys
from pathlib import Path
from playwright.sync_api import sync_playwright
root=Path(__file__).resolve().parents[1]; out=Path(sys.argv[1]); expected_commit=sys.argv[2]; base='http://127.0.0.1:18397'
env={'PATH':os.environ['PATH'],'HOME':os.environ['HOME'],'DATABASE_URL':'postgres://city_test:city_local_fixture@127.0.0.1:18732/city_test?sslmode=disable','REDIS_URL':'127.0.0.1:18579','JWT_SECRET':secrets.token_urlsafe(40),'PORT':'18397','TICK_SECONDS':'6','MAP_WIDTH':'30','MAP_HEIGHT':'20','WORLD_NAME':'City graphics acceptance','POLEIA_WORLD_START_WANAXES':'1','STATIC_DIR':str(root/'web/static'),'TEMPLATE_DIR':str(root/'web/templates'),'CHRONICLE_DIR':str(out/'chronicles'),'REPORTS_DIR':str(out/'reports')}
assert not (root/'server/.env').exists()
def api(path,method='GET',data=None,token=None):
 req=urllib.request.Request(base+path,method=method,data=None if data is None else json.dumps(data).encode(),headers={'Content-Type':'application/json',**({'Authorization':'Bearer '+token} if token else {})})
 with urllib.request.urlopen(req,timeout=15) as r:return json.load(r)
with (out/'acceptance-server.log').open('w') as log:
 proc=subprocess.Popen([str(out/'temenos')],cwd=root/'server',env=env,stdout=log,stderr=log)
 try:
  for i in range(60):
   try: health=api('/healthz'); break
   except Exception:
    if proc.poll() is not None:raise RuntimeError('game server exited; see acceptance-server.log')
    time.sleep(.5)
  else:raise RuntimeError('server timeout')
  assert health['commit']==expected_commit and health['migration']==158,health
  worlds=api('/api/v1/worlds'); world=(worlds if isinstance(worlds,list) else worlds['worlds'])[0]['id']
  token=api('/api/v1/auth/register','POST',{'username':'cityproof'+secrets.token_hex(4),'password':secrets.token_urlsafe(32)})['access_token']
  api('/api/v1/worlds/'+world+'/join','POST',{'province_name':'Courtyard'},token)
  founded=api('/api/v1/worlds/'+world+'/founding/settle','POST',{'name':'Courtyard'},token)
  assert founded.get('settlement_id')
  errors=[]
  with sync_playwright() as p:
   b=p.chromium.launch(); page=b.new_page(viewport={'width':1280,'height':900},device_scale_factor=1)
   page.on('pageerror',lambda e:errors.append(str(e)))
   page.goto(base+'/',wait_until='domcontentloaded')
   page.evaluate("t=>localStorage.setItem('poleia_token',t)",token)
   page.context.add_cookies([{'name':'poleia_token','value':token,'url':base}])
   page.goto(base+'/play',wait_until='networkidle')
   page.wait_for_function('window.openDrawer !== undefined')
   page.evaluate("window.openDrawer('city')")
   page.wait_for_selector('#city-scene')
   page.wait_for_selector('#city-gubbe-grid .gubbe-hex-g')
   page.locator('#drawer-city').screenshot(path=str(out/'acceptance-city-desktop.png'))
   page.set_viewport_size({'width':390,'height':844})
   page.locator('#drawer-city').screenshot(path=str(out/'acceptance-city-mobile.png'))
   canvas=page.locator('#city-scene').evaluate('(c)=>({width:c.width,height:c.height,cssWidth:c.getBoundingClientRect().width})')
   assert canvas['width']==320 and canvas['height']==152
   b.close()
  assert not errors,errors
  assets={}
  for f in ['city.js','citysprites.js']:
   content=urllib.request.urlopen(base+'/static/js/megaron/render/'+f).read()
   expected=(root/'web/static/js/megaron/render'/f).read_bytes();assert content==expected
   assets[f]=hashlib.sha256(content).hexdigest()
  proof={'health':health,'actual_player_found_city':True,'actual_drawer_and_grid':True,'screenshots':['acceptance-city-desktop.png','acceptance-city-mobile.png'],'browser_errors':errors,'assets_sha256':assets,'agora_configured':False}
  (out/'acceptance-proof.json').write_text(json.dumps(proof,indent=2)+'\n')
  print(json.dumps(proof))
 finally:
  proc.terminate(); proc.wait(timeout=15)
