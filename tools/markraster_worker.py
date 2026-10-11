#!/usr/bin/env python3
"""Locked-master pixel and main-thread render-block contract, Firefox first.
python3 tools/markraster_worker.py --mode before|after --host http://localhost:8750
The timed synchronous render task runs on the main thread in both engines.
Worker startup/arrival is included in settling, not attributed to main-thread blocking.
"""
import argparse, json, subprocess
from pathlib import Path
from playwright.sync_api import sync_playwright
from PIL import Image, ImageChops
p=argparse.ArgumentParser(); p.add_argument('--mode',choices=['before','after'],required=True); p.add_argument('--host',default='http://localhost:8750'); a=p.parse_args()
out=Path('docs/reviews/markraster-worker'); out.mkdir(parents=True,exist_ok=True)
base='547fd79e'
source=subprocess.check_output(['git','show',f'{base}:web/static/js/megaron/render/map.js']).decode() if a.mode=='before' else Path('web/static/js/megaron/render/map.js').read_text()
# No rendering arithmetic changes: the exported entry point gets a stopwatch.
source=source.replace('export function render() {','function measuredRender() {')+'''
export function render() { const start=performance.now(); try { return measuredRender(); }
finally { (window.__renderBlocks ||= []).push(performance.now()-start); } }
'''
results={'base':base,'mode':a.mode,'engines':{}}
with sync_playwright() as pw:
 for engine in ['firefox','chromium']:
  browser=getattr(pw,engine).launch(); res={}; results['engines'][engine]=res
  for label,rig,query in [('world','world',''),('forest','forest','zoom=1'),('relief','forest','fixture=relief&zoom=1'),('cities','cities',''),('river','forest','fixture=river&zoom=1'),('world-min','world','zoom=0.3'),('world-fow','world','fixture=fow')]:
   page=browser.new_page(viewport={'width':1600 if rig=='world' else 1040,'height':900},device_scale_factor=1); errors=[]
   page.add_init_script('''window.__taskGap=0;window.__lastTimer=performance.now();window.__resetGap=()=>{window.__taskGap=0;window.__lastTimer=performance.now()};setInterval(()=>{const n=performance.now();window.__taskGap=Math.max(window.__taskGap,n-window.__lastTimer-5);window.__lastTimer=n},5);''')
   page.on('pageerror',lambda e:errors.append(str(e))); page.on('console',lambda m:errors.append(m.text) if m.type=='error' else None)
   page.route('**/js/megaron/render/map.js',lambda route:route.fulfill(status=200,content_type='text/javascript',body=source))
   page.goto(a.host+f'/static/showcase-{rig}.html?'+query,wait_until='networkidle'); page.wait_for_function('window.SHOWCASE?.ready')
   # Rigs deliberately have no live RAF. Drive frozen frames until both viewport
   # and prefetch ring have settled, including asynchronous worker replies.
   page.evaluate('''async () => { const m=await import('/static/js/megaron/render/map.js'); window.__stats=m.markrasterStats;
    for(let n=0;n<3000;n++){ SHOWCASE.draw(); if(m.markrasterStats.pending===0 || (m.markrasterStats.pending===undefined && !((await import('/static/js/megaron/state.js')).State.dirty))) break; await new Promise(r=>setTimeout(r,16)); }
    SHOWCASE.draw(); }''')
   blocks=page.evaluate('window.__renderBlocks'); stats=page.evaluate('({...window.__stats})'); assert not stats.get('pending'),stats
   page.evaluate('()=>new Promise(r=>setTimeout(r,20))')
   res[label]={'first_view_max_render_block_ms':max(blocks),'first_view_max_timer_lateness_ms':page.evaluate('window.__taskGap'),'chunks':stats}
   if a.mode=='after':assert stats['mode']=='worker','supported browser must bake in worker'
   filename=f'{engine}-{label}-{a.mode}.png'; page.locator('#map-root').screenshot(path=str(out/filename))
   if a.mode=='after':
    d=ImageChops.difference(Image.open(out/filename).convert('RGB'),Image.open(out/f'{engine}-{label}-before.png').convert('RGB'))
    changed=sum(rgb!=(0,0,0) for rgb in d.getdata()); res[label]['changed_pixels']=changed
    assert changed==0,(engine,label,changed,d.getbbox())
   if label=='world':
    page.evaluate('''async()=> { window.__renderBlocks=[];window.__resetGap(); const {State}=await import('/static/js/megaron/state.js'); const {canvas,SCALE}=await import('/static/js/megaron/render/map.js');
     const old=State.camera.zoom*SCALE, x=(canvas.width/2-State.camera.x)/old,y=(canvas.height/2-State.camera.y)/old;
     State.camera.zoom=0.3; State.camera.x=canvas.width/2-x*0.3*SCALE;State.camera.y=canvas.height/2-y*0.3*SCALE;SHOWCASE.draw(); }''')
    page.evaluate('''async()=> { const {State}=await import('/static/js/megaron/state.js');for(let n=0;n<3000;n++){ SHOWCASE.draw(); if(window.__stats.pending===0 || (window.__stats.pending===undefined && !State.dirty))break;await new Promise(r=>setTimeout(r,16)); } }''')
    res[label]['zoom_out_max_render_block_ms']=max(page.evaluate('window.__renderBlocks'))
    page.evaluate('()=>new Promise(r=>setTimeout(r,20))');res[label]['zoom_out_max_timer_lateness_ms']=page.evaluate('window.__taskGap')
   assert not errors,errors; page.close(); print(engine,label,res[label],flush=True)
  browser.close()
(out/f'{a.mode}.json').write_text(json.dumps(results,indent=2)+'\n')
