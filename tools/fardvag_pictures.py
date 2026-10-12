#!/usr/bin/env python3
"""Firefox normal-scale movement BILD, real API projections and live FOW.
Baseline UI uses actual base-commit modules with current compatible party APIs;
original pre-change payloads/process are separately recorded by fixture.py.
"""
import argparse,json,pathlib,subprocess,urllib.parse,urllib.request
from playwright.sync_api import sync_playwright
ROOT=pathlib.Path(__file__).resolve().parents[1];OUT=ROOT/'docs/reviews/fardvag';BASE='http://localhost:8097'
from fardvag_fixture import api,credentials
parser=argparse.ArgumentParser();parser.add_argument('--baseline-ui',action='store_true');args=parser.parse_args()
folder=OUT/('baseline-ui' if args.baseline_ui else 'pictures');folder.mkdir(exist_ok=True)
s=json.loads((OUT/'scenario.json').read_text());token,viewer=credentials();health=json.load(urllib.request.urlopen(BASE+'/healthz'))
expected=subprocess.check_output(['git','-C',str(ROOT),'rev-parse','--short=7','HEAD'],text=True).strip()
assert health['commit']==expected and health['migration']==172,health
facts=[];cache={}
with sync_playwright() as pw:
 browser=pw.firefox.launch()
 for mode,(width,height) in {'desktop':(1280,900),'mobile':(390,844)}.items():
  ctx=browser.new_context(viewport={'width':width,'height':height},has_touch=mode=='mobile')
  ctx.add_cookies([{'name':'poleia_token','value':token,'url':BASE}]);ctx.add_init_script(f"localStorage.setItem('poleia_token',{json.dumps(token)})")
  if args.baseline_ui:
   def old_module(route):
    rel=urllib.parse.urlparse(route.request.url).path.removeprefix('/static/')
    if rel not in cache:cache[rel]=subprocess.check_output(['git','-C',str(ROOT),'show','746398ed:web/static/'+rel])
    route.fulfill(body=cache[rel],content_type='text/javascript')
   ctx.route('**/static/js/megaron/**',old_module)
  page=ctx.new_page();errors=[];page.on('pageerror',lambda e:errors.append(str(e)))
  page.goto(BASE+'/world/'+s['world']+'/map',wait_until='networkidle')
  page.evaluate("""async()=>{window.__s=(await import('/static/js/megaron/state.js')).State;window.__m=await import('/static/js/megaron/render/map.js');}""")
  page.wait_for_function('__s.tileData.length>0 && __m.markrasterStats.pending===0')
  # Freeze art gait/water frames only; server positions, payloads and FOW stay real.
  page.evaluate('()=>{window.requestAnimationFrame=()=>0;}')
  points=page.evaluate("""actors=>actors.map(a=>{let row,pos;if(a.kind==='army'||a.kind==='ship'){row=__s.unitsData.find(x=>x.id===a.id);pos={q:row.current_q??row.q,r:row.current_r??row.r};}else if(a.kind.startsWith('foreign-')){row=__s.foreignUnitData.find(x=>x.id===a.id);pos={q:row.q,r:row.r};}else if(a.kind.includes('runner')){row=__s.messengerData.find(x=>x.id===a.id);pos=row.current_q!=null?{q:row.current_q,r:row.current_r}:{q:Math.round((row.origin_q+row.dest_q)/2),r:Math.round((row.origin_r+row.dest_r)/2)};}else{row=__s.tradeData.find(x=>x.id===a.id);pos=row.current_q!=null?{q:row.current_q,r:row.current_r}:{q:Math.round((row.origin_q+row.dest_q)/2),r:Math.round((row.origin_r+row.dest_r)/2)};}return {...a,focus:pos};})""",s['actors'])
  for actor in points:
   page.evaluate('p=>centreOn(p.q,p.r)',actor['focus']);page.wait_for_function('__m.markrasterStats.pending===0')
   page.evaluate('__s.animFrame=0;__s.dirty=true;__m.render()')
   sample=page.evaluate("""id=>{const p=__m.journeyOverlays?__m.journeyOverlays().find(p=>p.actor_id===id):null;return {primitive:p,tile:__s.tileData.find(t=>t.q===(p?.q??0)&&t.r===(p?.r??0)),zoom:__s.camera.zoom,frame:__s.animFrame};}""",actor['id'])
   if not args.baseline_ui:
    wanted='route' if actor['own'] or actor['kind']=='agreed' else 'arrow'
    assert sample['primitive'] and sample['primitive']['type']==wanted,(actor,sample)
    q,r=actor['focus']['q'],actor['focus']['r'];tier=page.evaluate('h=>__s.tileData.find(t=>t.q===h.q&&t.r===h.r)?.tier',actor['focus']);assert tier=='live',(actor,tier)
   path=folder/f'{mode}-{actor["kind"]}.png';page.screenshot(path=str(path))
   facts.append({'mode':mode,'actor':actor,'url':page.url,'sample':sample,'errors':list(errors)})
  ctx.close()
 browser.close()
assert all(not f['errors'] for f in facts),facts
(folder/'facts.json').write_text(json.dumps({'health':health,'ui_source':'746398ed modules via browser interception' if args.baseline_ui else expected,'fixture':s['fixture'],'captures':facts},indent=2)+'\n')
print('Firefox1280+390:',len(facts),'captures; baseline UI' if args.baseline_ui else 'current live routes/arrows; no JS errors')
