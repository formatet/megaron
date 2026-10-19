#!/usr/bin/env python3
from pathlib import Path
import functools,http.server,json,threading
from playwright.sync_api import sync_playwright
HERE=Path(__file__).resolve().parent;ROOT=HERE.parents[2]
server=http.server.ThreadingHTTPServer(('127.0.0.1',0),functools.partial(http.server.SimpleHTTPRequestHandler,directory=str(ROOT)))
threading.Thread(target=server.serve_forever,daemon=True).start();errors=[];proof=[]
base=f'http://127.0.0.1:{server.server_port}/docs/reviews/palett-text/'
selectors=['.state-active','.march-support .intent-tag','.deposit-chip','.army-dp-val','.be-intent-support','.be-dir-out','.tr-bonus','.tr-eta','.tr-tribute-val','.mhs-rate','.mood-Favorable','.rel-ally','.sr-rate','.sb-rate','.food-state-growing']
try:
 with sync_playwright() as p:
  browser=p.firefox.launch();page=browser.new_page(viewport={'width':390,'height':844},device_scale_factor=1);page.on('pageerror',lambda e:errors.append(str(e)))
  data={}
  for mode in ['before','after','decision']:
   page.goto(base+'review.html?mode='+mode,wait_until='networkidle')
   colors=[]
   for selector in selectors:
    el=page.locator(selector);assert el.count()==1 and el.is_visible(),selector
    colors.append(el.evaluate('e=>{let s=getComputedStyle(e);let r=e.getBoundingClientRect();return {color:s.color,border:parseFloat(s.borderTopWidth)>0?s.borderColor:null,background:s.backgroundColor,x:r.x,y:r.y,w:r.width,h:r.height}}'))
   if mode=='after':assert all(c['color']=='rgb(120, 98, 0)' for c in colors),colors
   if mode=='before':assert all(c['color']=='rgb(212, 172, 13)' for c in colors),colors
   data[mode]=colors
   assert page.locator('.sample-dot').evaluate('e=>getComputedStyle(e).backgroundColor')=='rgb(212, 172, 13)'
   assert page.locator('.token-city').evaluate('e=>getComputedStyle(e).color')==('rgb(62, 107, 170)' if mode=='before' else 'rgb(53, 96, 149)')
   page.screenshot(path=str(HERE/('selectors-'+mode+'.png')),full_page=True)
   page.locator('#decision').screenshot(path=str(HERE/('decision-'+mode+'.png')))
   proof.append(dict(mode=mode,colors=colors,overflow=page.evaluate('document.documentElement.scrollWidth>innerWidth')))
  for a,b in zip(data['before'],data['after']):
   assert {k:v for k,v in a.items() if k!='color'}=={k:v for k,v in b.items() if k!='color'},('geometry/visible border/fill changed',a,b)
  browser.close()
 assert not errors,errors
 (HERE/'browser.json').write_text(json.dumps(dict(browser='Firefox; DPR1;390px;100% zoom',errors=errors,observations=proof),indent=2)+'\n')
finally:server.shutdown()
print('Firefox: all 15 CSS text selectors darkened; geometry/borders/fills identical; icons unchanged')
