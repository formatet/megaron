#!/usr/bin/env python3
"""Frozen real renderer in synthetic showcases; Firefox1:1 before/after.
No backend/gameplay claims. Baseline is an exact git source, supplied only as
renderer JavaScript; fixtures, camera, palette, seed and frozen frame are shared.
"""
from functools import partial
from http.server import ThreadingHTTPServer,SimpleHTTPRequestHandler
from pathlib import Path
from threading import Thread
from io import BytesIO
import argparse,hashlib,json,subprocess
from PIL import Image
from playwright.sync_api import sync_playwright
ROOT=Path(__file__).resolve().parents[3];OUT=Path(__file__).resolve().parent
p=argparse.ArgumentParser();p.add_argument('--before-only',action='store_true');args=p.parse_args()
BASELINE='2bf95ac7'
old=subprocess.check_output(['git','-C',str(ROOT),'show',BASELINE+':web/static/js/megaron/render/map.js'])
new=(ROOT/'web/static/js/megaron/render/map.js').read_bytes()
class Quiet(SimpleHTTPRequestHandler):
 def log_message(self,*args):pass
server=ThreadingHTTPServer(('127.0.0.1',0),partial(Quiet,directory=str(ROOT/'web')))
Thread(target=server.serve_forever,daemon=True).start();host=f'http://127.0.0.1:{server.server_port}'
scenes=[('olive-maquis','olive_maquis','1.0',False),('olive-maquis-min','olive_maquis','0.3',False),('olive-maquis-close','olive_maquis','1.6',False),('olive-maquis-gray','olive_maquis','1.0',True),('forest','forest','1.0',False),('cedar','cedar','1.0',False),('plains','plains','1.0',False),('no-olive','no_olive','1.0',False)]
metadata={'baseline_commit':BASELINE,'baseline_source_sha256':hashlib.sha256(old).hexdigest(),'current_source_sha256':hashlib.sha256(new).hexdigest(),'renderer_only':True,'scenes':{}}
def pixels(blob):return Image.open(BytesIO(blob)).convert('RGB').tobytes()
try:
 with sync_playwright() as pw:
  browser=pw.firefox.launch();metadata['firefox']=browser.version
  def shot(scene,label,source):
   name,fixture,zoom,gray=scene;page=browser.new_page(viewport={'width':1040,'height':900},device_scale_factor=1)
   errors=[];page.on('pageerror',lambda e:errors.append(str(e)))
   page.route('**/js/megaron/render/map.js',lambda route:route.fulfill(status=200,content_type='text/javascript',body=source))
   page.goto(host+f'/static/showcase-forest.html?fixture={fixture}&zoom={zoom}&world=olive-proof-fixed',wait_until='networkidle')
   page.wait_for_function('window.SHOWCASE?.ready')
   page.wait_for_function("async()=> (await import('/static/js/megaron/render/map.js')).markrasterStats.pending===0")
   page.evaluate('SHOWCASE.draw()')
   if gray:page.locator('#map-root').evaluate("el=>el.style.filter='grayscale(1)'")
   image=page.locator('#map-root').screenshot(animations='disabled')
   path=OUT/f'{name}-{label}.png'
   if label=='before' and path.exists():assert pixels(path.read_bytes())==pixels(image),'baseline changed '+name
   path.write_bytes(image)
   tiles=page.evaluate('SHOWCASE.tiles()');page.close();assert not errors,(name,errors)
   return image,tiles
  for scene in scenes:
   before,tiles=shot(scene,'before',old);row={'fixture':scene[1],'zoom':float(scene[2]),'grayscale':scene[3],'canvas_css':[900,780],'device_scale_factor':1,'tiles':tiles}
   if not args.before_only:
    after,_=shot(scene,'after',new);repeat,_=shot(scene,'repeat',new)
    b,a,r=map(pixels,[before,after,repeat]);assert a==r,scene[0]+' nondeterministic'
    changed=sum(a[i:i+3]!=b[i:i+3] for i in range(0,len(a),3));row['changed_pixels']=changed;row['repeat_changed_pixels']=0
    if scene[0]=='no-olive':assert changed==0,'change outside olive renderer'
    if scene[0]=='olive-maquis':assert changed>0,'no visible change'
   metadata['scenes'][scene[0]]=row
  browser.close()
 (OUT/('baseline.json' if args.before_only else 'proof.json')).write_text(json.dumps(metadata,indent=2))
 print(json.dumps({k:{kk:v for kk,v in row.items() if kk!='tiles'} for k,row in metadata['scenes'].items()},indent=2))
finally:server.shutdown();server.server_close()
