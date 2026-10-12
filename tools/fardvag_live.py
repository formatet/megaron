#!/usr/bin/env python3
"""Normal player march → actual route overlay → arrival, no SQL mutation."""
import json,os,pathlib,subprocess,tempfile,time,urllib.error
from playwright.sync_api import sync_playwright
from fardvag_fixture import api,BASE,PW,sql
ROOT=pathlib.Path(__file__).resolve().parents[1];OUT=ROOT/'docs/reviews/fardvag/live';OUT.mkdir(exist_ok=True)
USER='ParityNomad';token=api('auth/login',data={'username_or_email':USER,'password':PW})['access_token']
world=json.loads(sql("SELECT json_agg(t) FROM (SELECT id FROM worlds WHERE status='active' ORDER BY created_at DESC LIMIT 1) t"))[0]['id']
units=api(f'worlds/{world}/units',token)['units'];unit=next(u for u in units if u['type']=='nomadic_host' and u['status']=='positioned')
known=api(f'worlds/{world}/map',token)
candidates=[t for t in known if t['terrain'] not in {'fog','river','coastal_sea','deep_sea','mountain_limestone','mountain_red'} and 3<=max(abs(t['q']-unit['q']),abs(t['r']-unit['r']),abs(t['q']+t['r']-unit['q']-unit['r']))<=5]
preview=None;target=None
for t in candidates:
 try:p=api(f"worlds/{world}/units/{unit['id']}/march-preview?target_q={t['q']}&target_r={t['r']}",token)
 except urllib.error.HTTPError:continue
 if p.get('available') and 4<=p['duration_ticks']<=10:preview=p;target=t;break
assert target,'no normal known reachable 4–10 tick target'
with tempfile.TemporaryDirectory(prefix='fardvag-cli-') as tmp:
 tmp=pathlib.Path(tmp);binary=tmp/'keryx'
 subprocess.run(['go','build','-o',str(binary),'./cmd/keryx'],cwd=ROOT/'server',check=True)
 cfg=tmp/'config.json';cfg.write_text(json.dumps({'server':BASE,'token':token,'world_id':world,'username':USER}));cfg.chmod(0o600)
 env=dict(os.environ,POLEIA_CONFIG=str(cfg));env.pop('POLEIA_SERVER',None);env.pop('POLEIA_TOKEN',None)
 with (OUT/'sightings.txt').open('w') as log:subprocess.run([str(binary),'sightings'],env=env,stdout=log,stderr=subprocess.STDOUT,check=True)
 trace=[]
 with sync_playwright() as pw:
  browser=pw.firefox.launch();ctx=browser.new_context(viewport={'width':1280,'height':900});ctx.add_cookies([{'name':'poleia_token','value':token,'url':BASE}]);ctx.add_init_script(f"localStorage.setItem('poleia_token',{json.dumps(token)})")
  page=ctx.new_page();page.goto(BASE+'/world/'+world+'/map',wait_until='networkidle')
  page.evaluate("""async()=>{window.__s=(await import('/static/js/megaron/state.js')).State;window.__m=await import('/static/js/megaron/render/map.js');}""")
  page.wait_for_function('__s.tileData.length>0 && __m.markrasterStats.pending===0')
  order=api(f"worlds/{world}/units/{unit['id']}/march",token,{'target_q':target['q'],'target_r':target['r']})
  captured=False
  for _ in range(100):
   rows=api(f'worlds/{world}/units',token)['units'];u=next(u for u in rows if u['id']==unit['id']);trace.append({'status':u['status'],'q':u.get('current_q',u.get('q')),'r':u.get('current_r',u.get('r')),'path':u.get('path')})
   if not captured and u['status']=='marching':
    page.reload(wait_until='networkidle');page.evaluate("""async()=>{window.__s=(await import('/static/js/megaron/state.js')).State;window.__m=await import('/static/js/megaron/render/map.js');}""")
    page.wait_for_function('__s.tileData.length>0 && __m.markrasterStats.pending===0')
    page.evaluate('h=>centreOn(h.q,h.r)',{'q':u.get('current_q',u['q']),'r':u.get('current_r',u['r'])})
    marks=page.evaluate('__m.journeyOverlays()');mark=next((m for m in marks if m.get('actor_id')==unit['id']),None)
    if mark:
     assert mark['type']=='route';page.screenshot(path=str(OUT/'desktop-natural.png'))
     (OUT/'moving-unit.json').write_text(json.dumps({'unit':u,'primitive':mark},indent=2)+'\n');captured=True
   if captured and u['status']=='positioned' and u['q']==target['q'] and u['r']==target['r']:break
   time.sleep(.5)
  else:raise AssertionError('natural march/arrival was not completed')
  page.reload(wait_until='networkidle');page.evaluate("""async()=>{window.__m=await import('/static/js/megaron/render/map.js');}""")
  assert not any(m.get('actor_id')==unit['id'] for m in page.evaluate('__m.journeyOverlays()')),'arrived unit retained a forward route'
  browser.close()
 (OUT/'scenario.json').write_text(json.dumps({'world':world,'user':USER,'unit_before':unit,'target':{'q':target['q'],'r':target['r']},'preview':preview,'order':order,'trace':trace,'no_sql_mutation':True,'captured':captured,'arrival_route_absent':True},indent=2)+'\n')
print('normal march → LIVE route → arrival removes route; actual player/browser/CLI, no SQL writes')
