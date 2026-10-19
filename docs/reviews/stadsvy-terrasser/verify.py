#!/usr/bin/env python3
"""Production raster → exact PNGs and Firefox gallery at DPR1, 390/1440."""
from pathlib import Path
from PIL import Image,ImageOps,ImageChops
import base64,functools,hashlib,http.server,json,subprocess,threading
from playwright.sync_api import sync_playwright
HERE=Path(__file__).resolve().parent;ROOT=HERE.parents[2]
subprocess.run(['node','--experimental-default-type=module',str(HERE/'scenes.mjs')],cwd=ROOT,check=True,capture_output=True)
rows=json.loads((HERE/'scenes-raster.json').read_text())
for row in rows:
 stem=row['name'];r=row['raster']
 def image_layer(values):
  im=Image.new('RGBA',(160,96));im.putdata([tuple(bytes.fromhex(c[1:]))+(255,) if c else (0,0,0,0) for c in values]);return im
 structure=image_layer(r['structure']);structure.convert('RGB').save(HERE/(stem+'-structure.png'))
 life=image_layer(r['life']);status=image_layer(r['status'])
 life.save(HERE/(stem+'-life.png'));status.save(HERE/(stem+'-status.png'))
 mask=Image.new('L',(160,96));mask.putdata([255 if x else 0 for x in r['housing']]);mask.save(HERE/(stem+'-housing.png'))
 for state in ['construction','siege','starvation']:
  if stem==f'fixture-{row["sett"]["population"]}-{state}':
   base=next(s for s in rows if s['name']==f'fixture-{row["sett"]["population"]}-normal')
   assert r['structure']==base['raster']['structure'],'status baked into structure'
class Quiet(http.server.SimpleHTTPRequestHandler):
 def log_message(self,*args):pass
server=http.server.ThreadingHTTPServer(('127.0.0.1',0),functools.partial(Quiet,directory=str(ROOT)))
threading.Thread(target=server.serve_forever,daemon=True).start()
errors=[];observations=[]
try:
 with sync_playwright() as p:
  b=p.firefox.launch();page=b.new_page(viewport={'width':1440,'height':1000},device_scale_factor=1)
  page.on('pageerror',lambda error:errors.append(str(error)))
  page.goto(f'http://127.0.0.1:{server.server_port}/docs/reviews/stadsvy-terrasser/index.html',wait_until='networkidle');page.wait_for_function('window.galleryReady')
  assert page.locator('.viewport canvas').count()==len(rows)
  for row in rows:
   stem=row['name'];canvas=page.locator('#'+stem+' .viewport canvas')
   data=canvas.evaluate('c=>c.toDataURL().split(",")[1]');(HERE/(stem+'-2x.png')).write_bytes(base64.b64decode(data))
   im=Image.open(HERE/(stem+'-2x.png'))
   # Every physical 2×2 cell is one exact logical pixel, no interpolation.
   logical=im.resize((160,96),Image.Resampling.NEAREST)
   assert not ImageChops.difference(im,logical.resize((320,192),Image.Resampling.NEAREST)).getbbox()
   logical.save(HERE/(stem+'.png'));logical.resize((640,384),Image.Resampling.NEAREST).save(HERE/(stem+'-4x.png'))
   if row['group']=='dev':canvas.screenshot(path=str(HERE/(stem+'-firefox.png')))
  for width,height in [(1440,1000),(390,844)]:
   page.set_viewport_size({'width':width,'height':height});page.evaluate('scrollTo(0,0)')
   rects=page.locator('.viewport canvas').evaluate_all('(cs)=>cs.map(c=>{const r=c.getBoundingClientRect();return [r.width,r.height]})')
   assert all(r==[320,192] for r in rects),rects
   assert page.evaluate('document.documentElement.scrollWidth===innerWidth')
   page.screenshot(path=str(HERE/f'firefox-{width}.png'),full_page=True)
   page.locator('#gray').click();page.screenshot(path=str(HERE/f'firefox-{width}-gray.png'));page.locator('#gray').click()
   observations.append(dict(width=width,DPR=1,canvas_css=[320,192],overflow=False))
  for row in rows:
   if row['group']=='fixture' and row['name'].endswith('normal'):
    page.locator('#'+row['name']+' details').evaluate('(e)=>e.open=true')
  assert page.evaluate('document.documentElement.scrollWidth===innerWidth')
  b.close()
 assert not errors,errors
 # Compact comparison sheets, displayed at real game scale.
 for group in ['dev','fixture','walls']:
  selected=[r for r in rows if r['group']==group];cols=3 if group!='fixture' else 4
  sheet=Image.new('RGB',(cols*320,((len(selected)+cols-1)//cols)*192),'#EFE4CB')
  for i,r in enumerate(selected):sheet.paste(Image.open(HERE/(r['name']+'-2x.png')),(i%cols*320,i//cols*192))
  sheet.save(HERE/(group+'-contact.png'));ImageOps.grayscale(sheet).save(HERE/(group+'-gray.png'))
 proof=dict(browser='Firefox',observations=observations,page_errors=errors,scenes=len(rows),structural_status_separation=True,residential_pixels={r['name']:sum(r['raster']['housing']) for r in rows})
 (HERE/'verification.json').write_text(json.dumps(proof,indent=2)+'\n');print(json.dumps(proof))
finally:server.shutdown();server.server_close()
