#!/usr/bin/env python3
from pathlib import Path
import functools,hashlib,http.server,json,re,subprocess,threading
from playwright.sync_api import sync_playwright
HERE=Path(__file__).resolve().parent;ROOT=HERE.parents[2];css=ROOT/'web/static/megaron.css'
original=css.read_bytes();results=[]
for name,before,after,test in [('purple-danger',b'--danger:    #922B21;',b'--danger:    #6C3483;','approved danger is blood red'),('old-dim',b'--text-dim:  #615D56;',b'--text-dim:  #A04000;','approved muted text is warm grey'),('too-light-dim',b'--text-dim:  #615D56;',b'--text-dim:  #67635C;','approved muted text is warm grey')]:
 assert original.count(before)==1
 def run(phase):
  p=subprocess.run(['node','--experimental-default-type=module','--test','web/static/js/megaron/palette.test.mjs'],cwd=ROOT,capture_output=True,text=True)
  (HERE/(name+'-'+phase+'.log')).write_text(p.stdout+p.stderr);return p
 try:
  css.write_bytes(original.replace(before,after));red=run('red');assert red.returncode!=0 and re.search(r'not ok \d+ - '+test,red.stdout),(name,red.stdout)
 finally:css.write_bytes(original)
 assert css.read_bytes()==original
 assert run('restored-green').returncode==0
 results.append(dict(mutation=name,result='named RED -> byte-exact restore -> GREEN',source_sha256=hashlib.sha256(original).hexdigest()))
base=subprocess.check_output(['git','-C',str(ROOT),'show','584a324a:web/static/megaron.css'])
normalize=lambda b:re.sub(rb'^  --(?:text-dim|danger):[^\n]*\n',b'',b,flags=re.M)
assert normalize(base)==normalize(original),'CSS differs outside two token declarations'
(HERE/'mutations.json').write_text(json.dumps(results,indent=2)+'\n')
server=http.server.ThreadingHTTPServer(('127.0.0.1',0),functools.partial(http.server.SimpleHTTPRequestHandler,directory=str(ROOT)))
threading.Thread(target=server.serve_forever,daemon=True).start();observations=[];errors=[]
try:
 with sync_playwright() as p:
  browser=p.firefox.launch();page=browser.new_page(device_scale_factor=1);page.on('pageerror',lambda e:errors.append(str(e)))
  for scheme in ['light','dark']:
   page.emulate_media(color_scheme=scheme)
   for label,width,height in [('desktop',1280,900),('mobile',390,844)]:
    page.set_viewport_size(dict(width=width,height=height))
    page.goto(f'http://127.0.0.1:{server.server_port}/docs/reviews/palett-beslut/review.html',wait_until='networkidle')
    geometries=[]
    for arm in ['before','after']:
     page.evaluate("document.getElementById('old-palette')?.remove()")
     if arm=='before':page.add_style_tag(content=':root{--text-dim:#A04000;--danger:#6C3483}').evaluate("e=>e.id='old-palette'")
     data=page.locator('#decision').evaluate("""e=>Array.from(e.querySelectorAll('p,button')).map(n=>{let s=getComputedStyle(n),r=n.getBoundingClientRect();return {text:n.textContent,color:s.color,background:s.backgroundColor,x:r.x,y:r.y,width:r.width,height:r.height}})""")
     path=HERE/f'selector-{scheme}-{label}-{arm}.png';page.screenshot(path=str(path),full_page=True)
     again=page.screenshot(full_page=True);assert path.read_bytes()==again,'static screenshot nondeterministic'
     assert page.locator('.diplomacy').evaluate('e=>getComputedStyle(e).color')=='rgb(122, 74, 165)'
     assert page.locator('.state-collapsing').evaluate('e=>getComputedStyle(e).color')==('rgb(108, 52, 131)' if arm=='before' else 'rgb(146, 43, 33)')
     assert page.locator('.complete-at').first.evaluate('e=>getComputedStyle(e).color')==('rgb(160, 64, 0)' if arm=='before' else 'rgb(97, 93, 86)')
     geometries.append([{k:v for k,v in d.items() if k not in ['color','background']} for d in data])
     overflow=page.evaluate('document.documentElement.scrollWidth>innerWidth');assert not overflow
     observations.append(dict(scheme=scheme,width=width,arm=arm,selectors=data,overflow=overflow))
    assert geometries[0]==geometries[1],'geometry changed'
  browser.close()
 assert not errors
 (HERE/'browser.json').write_text(json.dumps(dict(browser='Firefox DPR1',errors=errors,css_contained_to_two_tokens=True,deterministic_static_screenshots=True,observations=observations),indent=2)+'\n')
finally:server.shutdown()
print('Three named mutations restored GREEN; only two CSS token declarations changed; Firefox light/dark 390/1280 geometry, diplomacy and repeat captures verified')
