#!/usr/bin/env python3
"""Real queue, resource and worker/pixel mutations, restore every edited byte."""
from pathlib import Path
import subprocess,json,argparse
from playwright.sync_api import sync_playwright
from PIL import Image,ImageChops
p=argparse.ArgumentParser();p.add_argument('--host',default='http://localhost:8750');a=p.parse_args()
out=Path('docs/reviews/markraster-worker'); results={}
unit='web/static/js/megaron/render/chunk_worker.test.mjs'
cases=[
 ('stale-scene','web/static/js/megaron/render/chunk_worker.js','data.generation === generation && data.generation === job.generation','true','late river/FOW/world result'),
 ('camera-priority','web/static/js/megaron/render/chunk_worker.js',"queue = jobs.filter(j => !(active?.generation === generation && j.key === active.key && j.sig === active.sig));",'queue = jobs;','worker queue keeps only one bake'),
 ('release-bitmaps','web/static/js/megaron/render/chunk_worker.js','} else dispose(data.chunk);','} else {}','late river/FOW/world result'),
]
for name,file,old,new,expected in cases:
 path=Path(file);original=path.read_bytes();s=original.decode();assert s.count(old)==1,name
 try:
  path.write_text(s.replace(old,new));r=subprocess.run(['node','--test',unit],capture_output=True);log=r.stdout+r.stderr
  (out/f'mutation-{name}.log').write_bytes(b'\n'.join(l.rstrip() for l in log.splitlines())+b'\n');named=next((l.decode() for l in log.splitlines() if l.startswith(b'not ok')), '')
  assert r.returncode and expected in named,(name,log.decode());results[name]=named;print(name,named,flush=True)
 finally:path.write_bytes(original)
 assert path.read_bytes()==original
with sync_playwright() as pw:
 browser=pw.firefox.launch()
 for name,file,old,new in [
  ('worker-disabled','web/static/js/megaron/render/map.js',"if (typeof Worker === 'undefined' || typeof OffscreenCanvas === 'undefined') return;",'if (true) return;'),
  ('pixel-rule','web/static/js/megaron/render/markraster.js','const WARP_AMP = 13;','const WARP_AMP = 0;'),
 ]:
  path=Path(file);original=path.read_bytes();s=original.decode();assert s.count(old)==1,name
  try:
   path.write_text(s.replace(old,new));page=browser.new_page(viewport={'width':1040,'height':900})
   page.goto(a.host+'/static/showcase-forest.html?zoom=1',wait_until='networkidle');page.wait_for_function('SHOWCASE.ready')
   stats=page.evaluate('''async()=>{const m=await import('/static/js/megaron/render/map.js');for(let n=0;n<1500;n++){SHOWCASE.draw();if(m.markrasterStats.pending===0)break;await new Promise(r=>setTimeout(r,16));}SHOWCASE.draw();return {...m.markrasterStats};}''')
   if name=='worker-disabled':
    try:assert stats['mode']=='worker','supported browser must bake in worker'
    except AssertionError as e:results[name]=f'RED: {e} (got {stats["mode"]})'
    else:raise AssertionError('worker-disabled mutation survived')
   else:
    image=out/'mutation-pixel-rule.png';page.locator('#map-root').screenshot(path=str(image))
    d=ImageChops.difference(Image.open(image).convert('RGB'),Image.open(out/'firefox-forest-before.png').convert('RGB'))
    changed=sum(v!=(0,0,0) for v in d.getdata())
    try:assert changed==0,'bitidentical forest pixel contract'
    except AssertionError as e:results[name]=f'RED: {e} ({changed} changed pixels)'
    else:raise AssertionError('pixel-rule mutation survived')
   page.close();print(name,results[name],flush=True)
  finally:path.write_bytes(original)
  assert path.read_bytes()==original
 browser.close()
r=subprocess.run(['node','--test',unit],capture_output=True);(out/'restored.log').write_bytes(r.stdout+r.stderr);assert r.returncode==0
results['restore']='all edited production files restored byte-identically; targeted green'
(out/'mutations.json').write_text(json.dumps(results,indent=2)+'\n')
