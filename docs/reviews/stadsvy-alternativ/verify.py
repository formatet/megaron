#!/usr/bin/env python3
"""Artifact checks and Firefox 100% review captures; no game-server acceptance claim."""
from pathlib import Path
from PIL import Image, ImageChops
import functools
import hashlib
from http.server import SimpleHTTPRequestHandler, ThreadingHTTPServer
import json
import subprocess
import sys
import threading
from playwright.sync_api import sync_playwright

HERE=Path(__file__).resolve().parent
ROOT=HERE.parents[2]

def check():
    manifest=json.loads((HERE/'manifest.json').read_text())
    assert len({x['id'] for x in manifest['images']})>=8
    for ident in manifest['favourites']:
        assert {x['state'] for x in manifest['images'] if x['id']==ident}=={'normal','starvation','siege','construction'}
    files=[p for p in HERE.glob('*.png') if p.name in json.loads((HERE/'png-sha256.json').read_text())]
    before={p.name:hashlib.sha256(p.read_bytes()).hexdigest() for p in files}
    subprocess.run([sys.executable,str(HERE/'generate.py')],check=True)
    after={p.name:hashlib.sha256(p.read_bytes()).hexdigest() for p in files}
    assert before==after,'non-deterministic generator'
    for row in manifest['images']:
        path=HERE/row['file'];im=Image.open(path).convert('RGB')
        assert list(im.size)==row['size']
        big=Image.open(path.with_stem(path.stem+'-4x')).convert('RGB')
        assert big.size==(im.width*4,im.height*4)
        assert ImageChops.difference(big,im.resize(big.size,Image.Resampling.NEAREST)).getbbox() is None,'nearest-neighbour scaling changed pixels'
        if row['id'] in manifest['favourites']:
            structure=Image.open(HERE/(row['id']+'-structure.png')).convert('RGBA')
            layer=Image.open(path.with_stem(path.stem+'-layer')).convert('RGBA')
            composite=Image.alpha_composite(structure,layer).convert('RGB')
            assert ImageChops.difference(im,composite).getbbox() is None,'status was baked into structure'
            normal=Image.open(HERE/(row['id']+'-normal.png')).convert('RGB')
            if row['state']!='normal':assert ImageChops.difference(im,normal).getbbox(),f"{row['id']} {row['state']} is visually identical to normal"
    return manifest,{'deterministic_pngs':len(before),'exact_nearest_neighbour_scene_pairs':len(manifest['images']),'status_layer_composites':8}

class Quiet(SimpleHTTPRequestHandler):
    def log_message(self,*args):pass

def browser():
    server=ThreadingHTTPServer(('127.0.0.1',0),functools.partial(Quiet,directory=str(ROOT)))
    threading.Thread(target=server.serve_forever,daemon=True).start()
    base=f'http://127.0.0.1:{server.server_port}'
    errors=[];result={}
    try:
        with sync_playwright() as p:
            b=p.firefox.launch();result['browser']=b.version
            page=b.new_page(viewport={'width':1440,'height':1000},device_scale_factor=1)
            page.on('pageerror',lambda e:errors.append(str(e)))
            page.goto(base+'/docs/reviews/stadsvy-alternativ/index.html',wait_until='networkidle')
            assert page.locator('figure').count()==20
            assert page.locator('img').evaluate_all('(xs)=>xs.every(x=>x.complete && x.naturalWidth>0)')
            sizes=page.locator('.logical img').evaluate_all('(xs)=>xs.map(x=>({natural:x.naturalWidth,width:x.getBoundingClientRect().width,scale:devicePixelRatio}))')
            assert all(x['natural']==x['width'] and x['scale']==1 for x in sizes)
            page.screenshot(path=str(HERE/'firefox-desktop.png'),full_page=True)
            page.locator('#enlarge').click()
            assert page.locator('details[open]').count()==20
            page.locator('#gray').click()
            assert page.locator('body.gray').count()==1
            assert page.locator('figure img').evaluate_all('(xs)=>xs.every(x=>getComputedStyle(x).filter==="grayscale(1)")')
            page.locator('#alternativ').scroll_into_view_if_needed()
            page.screenshot(path=str(HERE/'firefox-4x-grayscale.png'))
            page.locator('#enlarge').click();page.locator('#gray').click()
            page.set_viewport_size({'width':390,'height':844});page.evaluate('scrollTo(0,0)')
            assert page.evaluate('document.documentElement.scrollWidth<=innerWidth')
            page.screenshot(path=str(HERE/'firefox-mobile.png'),full_page=True)
            page.locator('#enlarge').click()
            assert page.evaluate('document.documentElement.scrollWidth<=innerWidth')
            page.locator('details').first.scroll_into_view_if_needed()
            page.screenshot(path=str(HERE/'firefox-mobile-4x.png'))
            # Baseline: invoke original public renderer with the exact same synthetic fixture.
            page.set_viewport_size({'width':400,'height':220})
            page.goto(base+'/web/static/',wait_until='domcontentloaded')
            page.set_content('<canvas id="baseline"></canvas>')
            page.evaluate('''async()=>{const m=await import('/web/static/js/megaron/render/city.js');
              m.startCityAnim(document.querySelector('canvas'),{terrain:'scrub_maquis'},
                [{type:'temple',level:1},{type:'farm',level:2},{type:'harbour',level:1},{type:'shipyard',level:1},{type:'mine',level:1}],[],{population:4500,walls:1});
              await new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r))); m.stopCityAnim();}''')
            page.locator('canvas').screenshot(path=str(HERE/'baseline-live-renderer.png'))
            b.close()
        assert not errors,errors
        result.update(logical_images_at_exact_css_size=len(sizes),page_errors=errors,mobile_width=390,all_4x_expand=True,grayscale_toggle=True,baseline='synthetic fixture in unchanged production renderer; not a live city or server')
        return result
    finally:server.shutdown();server.server_close()

if __name__=='__main__':
    manifest,result=check();result['firefox']=browser()
    (HERE/'verification.json').write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n')
    print(json.dumps(result,ensure_ascii=False))
