#!/usr/bin/env python3
"""Real worker, feature/failure fallbacks, FOW refresh and salt/cultivation lifecycle.
Synthetic showcase scenes; does not claim acceptance in the live game (slice C).
"""
import argparse,json
from pathlib import Path
from PIL import Image,ImageChops
from playwright.sync_api import sync_playwright
p=argparse.ArgumentParser();p.add_argument('--host',default='http://localhost:8750');a=p.parse_args()
out=Path('docs/reviews/markraster-worker');results={}
settle='''async()=> { const m=await import('/static/js/megaron/render/map.js');
for(let n=0;n<1500;n++){SHOWCASE.draw();if(m.markrasterStats.pending===0)break;await new Promise(r=>setTimeout(r,16));}
SHOWCASE.draw(); return {...m.markrasterStats}; }'''
# Change a compact scene while first-view chunks are still outstanding. River,
# remembered mask, unknown fog, city-driven parcels and world salt all change.
change='''async()=>{const {State}=await import('/static/js/megaron/state.js');
State.tileData=State.tileData.map(t=>({...t, terrain:t.q===0?'fog':t.terrain,tier:t.q===-1?'remembered':t.tier}));
State.provinceData=[...State.provinceData,{q:1,r:1}];State.WORLD_ID='worker-lifecycle-Tiryns';SHOWCASE.draw();}'''
with sync_playwright() as pw:
 for engine in ['firefox','chromium']:
  browser=getattr(pw,engine).launch();res={};results[engine]=res
  for mode in ['worker','no-offscreen','constructor-failure','runtime-failure']:
   page=browser.new_page(viewport={'width':1040,'height':900});errors=[]
   page.on('pageerror',lambda e:errors.append(str(e)));page.on('console',lambda m:errors.append(m.text) if m.type=='error' else None)
   if mode=='no-offscreen':page.add_init_script('window.OffscreenCanvas=undefined;')
   if mode=='constructor-failure':page.add_init_script('window.Worker=class{constructor(){throw new Error("worker blocked")}};')
   if mode=='runtime-failure':
    page.add_init_script('''window.Worker=class{postMessage(){setTimeout(()=>this.onerror?.(),0)} terminate(){}};''')
   # No networkidle wait: change the scene immediately after the rig's first
   # render, before a worker chunk can complete. This exercises superseded jobs.
   page.goto(a.host+'/static/showcase-forest.html?fixture=river&zoom=1',wait_until='load');page.wait_for_function('window.SHOWCASE?.ready')
   await_count=page.evaluate('''async()=>{ const m=await import('/static/js/megaron/render/map.js'); return m.markrasterStats.pending; }''')
   if mode=='worker':assert await_count>0,'first view must enqueue instead of synchronously baking'
   page.evaluate(change);stats=page.evaluate(settle);assert stats['pending']==0,stats
   assert stats['mode']==('worker' if mode=='worker' else 'budget'),stats
   page.locator('#map-root').screenshot(path=str(out/f'{engine}-lifecycle-{mode}.png'))
   res[mode]={'stats':stats,'errors':errors}; assert not errors,errors
   # Refresh of equal-content arrays must not re-bake accepted chunks.
   before=stats['chunks'];page.evaluate('''async()=>{const {State}=await import('/static/js/megaron/state.js'); State.tileData=State.tileData.map(t=>({...t})); State.provinceData=State.provinceData.map(p=>({...p}));SHOWCASE.draw();}''')
   refreshed=page.evaluate(settle);assert refreshed['chunks']==before,'unchanged signatures must reuse cached chunks'
   page.close()
  for mode in ['no-offscreen','constructor-failure','runtime-failure']:
   d=ImageChops.difference(Image.open(out/f'{engine}-lifecycle-worker.png').convert('RGB'),Image.open(out/f'{engine}-lifecycle-{mode}.png').convert('RGB'))
   count=sum(v!=(0,0,0) for v in d.getdata());res[mode]['changed_pixels_vs_worker']=count;assert count==0,(engine,mode,count,d.getbbox())
  # Old polygon renderer is still callable after extracting its ground code.
  page=browser.new_page();page.goto(a.host+'/static/showcase-forest.html?fixture=relief&markraster=0',wait_until='networkidle');page.wait_for_function('SHOWCASE.ready');page.evaluate('SHOWCASE.draw()');page.close()
  browser.close();print(engine,res,flush=True)
(out/'lifecycle.json').write_text(json.dumps(results,indent=2)+'\n')
