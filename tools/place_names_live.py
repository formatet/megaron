#!/usr/bin/env python3
"""Normal new Wanax → march → first-reach → web naming + real CLI.
No SQL changes. Uses a separate account and finds a currently known, walkable,
unnamed natural object through the actual map and march-preview APIs.
"""
import json,os,pathlib,subprocess,tempfile,time,urllib.error,urllib.request
from playwright.sync_api import sync_playwright
ROOT=pathlib.Path(__file__).resolve().parents[1];OUT=ROOT/'docs/reviews/flod-doparratt/live';OUT.mkdir(parents=True,exist_ok=True);BASE='http://localhost:8097';PW='acceptance-pw-123';USER=os.environ.get('PLACE_PROOF_USER','PlaceProofDoria')
def api(path,token=None,data=None):
 req=urllib.request.Request(BASE+'/api/v1/'+path,headers={'Content-Type':'application/json',**({'Authorization':'Bearer '+token} if token else {})},data=None if data is None else json.dumps(data).encode());return json.load(urllib.request.urlopen(req,timeout=30))
health=json.load(urllib.request.urlopen(BASE+'/healthz'));world=api('worlds')[0]['id']
try:token=api('auth/login',data={'username_or_email':USER,'password':PW})['access_token']
except urllib.error.HTTPError:
 token=api('auth/register',data={'username':USER,'password':PW})['access_token'];api(f'worlds/{world}/join',token,{})
status=api(f'worlds/{world}/founding/status',token);assert status['active'],status
places=api(f'worlds/{world}/places',token)['places'];tiles=api(f'worlds/{world}/map',token);host=status['host_unit_id']
candidates=[t for t in tiles if t.get('place_kind') in ('river','desert','forest') and not t.get('place_name') and t['terrain'] not in ('river','fog') and (t['q'],t['r'])!=(status['q'],status['r'])]
candidates.sort(key=lambda t:max(abs(t['q']-status['q']),abs(t['r']-status['r']),abs(t['q']+t['r']-status['q']-status['r'])))
target=None
resume=os.environ.get('PLACE_PROOF_RESUME')=='1'
if resume:
 claimed=next(p for p in places if p.get('can_name'))
 target=next(t for t in tiles if t.get('place_kind')==claimed['kind'] and t.get('place_id')==claimed['id'] and t['terrain'] not in ('river','fog'))
 before=api(f'worlds/{world}/places',token);preview=None;order={'resume':'previous normal API march; see live-host-reached.json and DB route proof'};trace=[]
else:
 target=None
 for t in candidates:
  try:
   preview=api(f"worlds/{world}/units/{host}/march-preview?target_q={t['q']}&target_r={t['r']}",token)
   if preview.get('available') and preview['duration_ticks']<=12:target=t;break
  except urllib.error.HTTPError:pass
 assert target,'new normal account has no reachable known unnamed object; use another account/world'
 assert not target.get('can_name'),'physical entry proof must start without right'
 before=api(f'worlds/{world}/places',token)
 order=api(f'worlds/{world}/units/{host}/march',token,{'target_q':target['q'],'target_r':target['r']})
 trace=[];claimed=None
 for _ in range(100):
  data=api(f'worlds/{world}/places',token);s=api(f'worlds/{world}/founding/status',token);trace.append({'tick':s['current_tick'],'q':s.get('q'),'r':s.get('r'),'places':data})
  claimed=next((p for p in data['places'] if p['kind']==target['place_kind'] and p['id']==target['place_id'] and p.get('can_name')),None)
  if claimed:break
  time.sleep(1)
 assert claimed,'normal march failed to obtain first-reach right'
notices=api(f'worlds/{world}/notifications',token)
other=api('auth/login',data={'username_or_email':'Eirene','password':PW})['access_token']
refusals=[]
for auth_token,name,expected in [(other,'Other Name',403),(token,'x',400)]:
 try:api(f"worlds/{world}/places/{claimed['kind']}/{claimed['id']}/name",auth_token,{'name':name});raise AssertionError('invalid naming request succeeded')
 except urllib.error.HTTPError as e:
  body=e.read().decode();assert e.code==expected,(e.code,body);refusals.append({'status':e.code,'body':body})
(OUT/'refusals.json').write_text(json.dumps(refusals,indent=2)+'\n')

# Private temporary CLI config, never exported to proof artifacts.
with tempfile.TemporaryDirectory(prefix='place-live-') as tmp:
 tmp=pathlib.Path(tmp);binary=tmp/'keryx';subprocess.run(['go','build','-o',str(binary),'./cmd/keryx'],cwd=ROOT/'server',check=True)
 cfg=tmp/'config.json';cfg.write_text(json.dumps({'server':BASE,'token':token,'world_id':world,'username':USER}));cfg.chmod(0o600);env=dict(os.environ,POLEIA_CONFIG=str(cfg));env.pop('POLEIA_SERVER',None);env.pop('POLEIA_TOKEN',None)
 def cli(args,file,expected=0):
  p=subprocess.run([str(binary),*args],env=env,capture_output=True,text=True);(OUT/file).write_text(p.stdout+p.stderr);assert p.returncode==expected,(args,p.stdout,p.stderr);return p.stdout
 cli(['places'],'places-before.txt');cli(['notifications'],'notifications.txt')
 with sync_playwright() as pw:
  browser=pw.firefox.launch()
  for mode,(w,h) in {'desktop':(1280,900),'mobile':(390,844)}.items():
   ctx=browser.new_context(viewport={'width':w,'height':h},has_touch=mode=='mobile');ctx.add_cookies([{'name':'poleia_token','value':token,'url':BASE}]);ctx.add_init_script(f"localStorage.setItem('poleia_token',{json.dumps(token)})");page=ctx.new_page();errors=[];page.on('pageerror',lambda e:errors.append(str(e)))
   page.goto(BASE+f'/world/{world}/map?ortnamn=a',wait_until='networkidle');page.evaluate("async()=>{window.__ls=(await import('/static/js/megaron/state.js')).State;window.__lm=await import('/static/js/megaron/render/map.js');}");page.wait_for_function('__ls.tileData.length>0 && __lm.markrasterStats.pending===0');page.evaluate('t=>centreOn(t.q,t.r)',target);page.wait_for_timeout(500)
   point=page.evaluate('t=>{const p=__lm.hexPx(t.q,t.r);return {x:__ls.camera.x+p.x*__lm.SCALE*__ls.camera.zoom,y:__ls.camera.y+p.y*__lm.SCALE*__ls.camera.zoom}}',target);el=page.locator('#hex-canvas');el.tap(position=point) if mode=='mobile' else el.click(position=point)
   page.locator('[aria-label="Place name"]').wait_for();page.screenshot(path=str(OUT/f'{mode}-natural-form.png'))
   if mode=='mobile':
    page.locator('[aria-label="Place name"]').fill('Doria');page.locator('.place-note button').filter(has_text='Name it').click();page.locator('[aria-label="Place name"]').wait_for(state='hidden');page.screenshot(path=str(OUT/'mobile-natural-named-panel.png'));page.evaluate('__lm.closeInspect()');page.wait_for_timeout(500);page.screenshot(path=str(OUT/'mobile-natural-named-map.png'))
   assert not errors,errors;ctx.close()
  browser.close()
 cli(['places'],'places-after.txt');cli(['places','name',claimed['kind'],claimed['id'],'Second Try'],'second-name-409.txt',expected=1)
(OUT/'scenario.json').write_text(json.dumps({'health':health,'world':world,'account':USER,'setup':'normal APIs only; no SQL','start':status,'before':before,'target':target,'preview':preview,'order':order,'trace':trace,'claimed':claimed,'notices':notices,'after':api(f'worlds/{world}/places',token)},indent=2)+'\n')
print('normal new player march → claim → native Firefox form → name; real CLI list/notices/409')
