#!/usr/bin/env python3
"""Artifact dependencies and Firefox review; human size recognition remains BILD."""
from pathlib import Path
from PIL import Image,ImageChops
import functools,hashlib,json,subprocess,sys,threading
from http.server import SimpleHTTPRequestHandler,ThreadingHTTPServer
from playwright.sync_api import sync_playwright
HERE=Path(__file__).resolve().parent;ROOT=HERE.parents[3]

def check():
 hashes=json.loads((HERE/'png-sha256.json').read_text())
 subprocess.run([sys.executable,str(HERE/'generate.py')],check=True,capture_output=True)
 assert hashes==json.loads((HERE/'png-sha256.json').read_text()),'non-deterministic PNG'
 data=json.loads((HERE/'manifest.json').read_text());rows=data['scenes'];assert len(rows)==9
 for r in rows:
  stem=r['stem'];im=Image.open(HERE/(stem+'.png')).convert('RGB');assert im.size==(160,96)
  for scale in [2,4]:
   actual=Image.open(HERE/(stem+f'-{scale}x.png')).convert('RGB')
   assert ImageChops.difference(actual,im.resize((160*scale,96*scale),Image.Resampling.NEAREST)).getbbox() is None,'nearest-neighbour raster mismatch'
  structure=Image.open(HERE/(stem+'-structure.png')).convert('RGBA');life=Image.open(HERE/(stem+'-life.png')).convert('RGBA')
  assert ImageChops.difference(im,Image.alpha_composite(structure,life).convert('RGB')).getbbox() is None,'life baked into structure'
  mask=Image.open(HERE/(stem+'-housing-mask.png'))
  assert sum(1 for v in mask.get_flattened_data() if v)==r['housing_pixels'],'residential mask count miscalibrated'
  assert r['housing_pixels']>0
 for method in ['gatuklyfta','terrasser','bortom-templet']:
  group=sorted([r for r in rows if r['method']==method],key=lambda r:r['population'])
  assert [r['population'] for r in group]==[1000,4500,10000]
  assert all(r['walls']==1 for r in group)
  pixels=[r['housing_pixels'] for r in group]
  assert pixels[0]<pixels[1]<pixels[2],method+' residential mass no longer grows'
  bare=[Image.open(HERE/(r['stem']+'-structure.png')).tobytes() for r in group]
  assert len(set(bare))==3,method+' structure no longer depends on population'
  horizons=[Image.open(HERE/(r['stem']+'-structure.png')).getpixel((80,11)) for r in group]
  assert len(set(horizons))==1,'mountain sample closed by population'
 return dict(generated_pngs=len(hashes),scenes=9,exact_scaled_pairs=18,life_composites=9,residential_mask_counts='nonzero binary pixels; structural residential paint only',structure_without_figures='three distinct sizes per method',mountain_sample='same pixel(80,11) across sizes; sanity check, not recognition proof')
class Quiet(SimpleHTTPRequestHandler):
 def log_message(self,*args):pass

def browser():
 server=ThreadingHTTPServer(('127.0.0.1',0),functools.partial(Quiet,directory=str(ROOT)));threading.Thread(target=server.serve_forever,daemon=True).start();errors=[]
 try:
  with sync_playwright() as p:
   b=p.firefox.launch();page=b.new_page(viewport={'width':1440,'height':1000},device_scale_factor=1)
   page.on('pageerror',lambda e:errors.append(str(e)))
   page.goto(f'http://127.0.0.1:{server.server_port}/docs/reviews/stadsvy-alternativ/21c/index.html',wait_until='networkidle')
   assert page.locator('figure').count()==9
   assert page.locator('img').evaluate_all('(xs)=>xs.every(x=>x.complete&&x.naturalWidth>0)')
   assert page.locator('.viewport img').evaluate_all('(xs)=>xs.every(x=>x.getBoundingClientRect().width===320&&x.getBoundingClientRect().height===192&&devicePixelRatio===1)')
   page.screenshot(path=str(HERE/'firefox-desktop.png'),full_page=True)
   page.locator('#gray').click();assert page.locator('figure img').evaluate_all('(xs)=>xs.every(x=>getComputedStyle(x).filter==="grayscale(1)")')
   page.screenshot(path=str(HERE/'firefox-grayscale.png'));page.locator('#gray').click()
   page.set_viewport_size({'width':390,'height':844});page.evaluate('scrollTo(0,0)')
   assert page.evaluate('document.documentElement.scrollWidth===innerWidth')
   page.screenshot(path=str(HERE/'firefox-mobile.png'),full_page=True)
   page.locator('details').first.locator('summary').click()
   assert page.evaluate('document.documentElement.scrollWidth===innerWidth')
   page.screenshot(path=str(HERE/'firefox-mobile-expanded.png'))
   result=dict(browser=b.version,DPR=1,viewports=[1440,390],image_css=[320,192],horizontal_overflow=False,page_errors=errors)
   b.close()
  assert not errors,errors
  return result
 finally:server.shutdown();server.server_close()
if __name__=='__main__':
 result=check();result['firefox']=browser();(HERE/'verification.json').write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n');print(json.dumps(result,ensure_ascii=False))
