#!/usr/bin/env python3
"""Check parameterized scenes, status compositing and real 390px panel passform."""
from pathlib import Path
from PIL import Image,ImageChops
import functools,hashlib,json,subprocess,sys,threading
from http.server import SimpleHTTPRequestHandler,ThreadingHTTPServer
from playwright.sync_api import sync_playwright
HERE=Path(__file__).resolve().parent
ROOT=HERE.parents[3]

def check():
    hashes=json.loads((HERE/'png-sha256.json').read_text())
    subprocess.run([sys.executable,str(HERE/'generate.py')],check=True,capture_output=True)
    assert hashes==json.loads((HERE/'png-sha256.json').read_text()),'generation is not deterministic'
    manifest=json.loads((HERE/'manifest.json').read_text());rows=manifest['scenes']
    assert len(rows)==28
    for row in rows:
        stem=row['stem'];im=Image.open(HERE/(stem+'.png')).convert('RGB');assert im.size==(160,96)
        for scale in [2,4]:
            actual=Image.open(HERE/(stem+f'-{scale}x.png')).convert('RGB')
            assert ImageChops.difference(actual,im.resize((160*scale,96*scale),Image.Resampling.NEAREST)).getbbox() is None,'nearest-neighbour scaling changed pixels'
        structure=Image.open(HERE/(stem+'-structure.png')).convert('RGBA')
        layer=Image.open(HERE/(stem+'-layer.png')).convert('RGBA')
        assert set(layer.getchannel('A').get_flattened_data())<={0,255},'partial alpha in pixel layer'
        assert ImageChops.difference(im,Image.alpha_composite(structure,layer).convert('RGB')).getbbox() is None
    for camera in ['gata','tak']:
        for walls in [0,1,3]:
            ims=[Image.open(HERE/f'{camera}-p{pop}-m{walls}-normal-huvud.png').tobytes() for pop in [1000,4500,10000]]
            assert len(set(ims))==3,f'{camera} population stopped affecting the image at wall {walls}'
        for pop in [1000,4500,10000]:
            ims=[Image.open(HERE/f'{camera}-p{pop}-m{wall}-normal-huvud.png').tobytes() for wall in [0,1,3]]
            assert len(set(ims))==3,f'{camera} walls stopped affecting the image at population {pop}'
        structural=[Image.open(HERE/f'{camera}-p4500-m1-{state}-huvud-structure.png').tobytes() for state in ['normal','starvation','siege','construction']]
        assert len(set(structural))==1,'status changed the city structure'
        states=[Image.open(HERE/f'{camera}-p4500-m1-{state}-huvud.png').tobytes() for state in ['normal','starvation','siege','construction']]
        assert len(set(states))==4,f'{camera} status distinction lost'
    return dict(deterministic_generated_pngs=len(hashes),scenes=len(rows),exact_integer_scaled_pairs=len(rows)*2,exact_status_composites=len(rows),population_wall_matrix='18 combinations; pixel differences prove dependency, human readability pending BILD')

class Quiet(SimpleHTTPRequestHandler):
    def log_message(self,*args):pass

def browser():
    server=ThreadingHTTPServer(('127.0.0.1',0),functools.partial(Quiet,directory=str(ROOT)))
    threading.Thread(target=server.serve_forever,daemon=True).start();errors=[];result={}
    try:
        with sync_playwright() as p:
            b=p.firefox.launch();page=b.new_page(viewport={'width':1440,'height':1000},device_scale_factor=1)
            page.on('pageerror',lambda e:errors.append(str(e)))
            page.goto(f'http://127.0.0.1:{server.server_port}/docs/reviews/stadsvy-alternativ/21b/index.html',wait_until='networkidle')
            assert page.locator('figure').count()==32
            assert page.locator('img').evaluate_all('(xs)=>xs.every(x=>x.complete && x.naturalWidth>0)')
            assert page.locator('.viewport img').evaluate_all('(xs)=>xs.every(x=>x.getBoundingClientRect().width===320 && x.getBoundingClientRect().height===192 && devicePixelRatio===1)')
            page.screenshot(path=str(HERE/'firefox-desktop.png'),full_page=True)
            page.locator('#gray').click()
            assert page.locator('figure img').evaluate_all('(xs)=>xs.every(x=>getComputedStyle(x).filter==="grayscale(1)")')
            page.screenshot(path=str(HERE/'firefox-gray.png'))
            page.locator('#gray').click();page.set_viewport_size({'width':390,'height':844});page.evaluate('scrollTo(0,0)')
            assert page.evaluate('document.documentElement.scrollWidth===innerWidth')
            rects=page.locator('.viewport').evaluate_all('(xs)=>xs.map(x=>({width:x.clientWidth,image:x.querySelector("img").getBoundingClientRect().width}))')
            assert all(x['width']>=320 and x['image']==320 for x in rects)
            page.screenshot(path=str(HERE/'firefox-mobile.png'),full_page=True)
            page.locator('details').first.locator('summary').click()
            assert page.evaluate('document.documentElement.scrollWidth===innerWidth')
            page.screenshot(path=str(HERE/'firefox-mobile-expanded.png'))
            # A literal 390px panel with 24px interior margins and 1px border: no fitting by fractional resize.
            page.set_content('<div id="panel" style="width:390px;box-sizing:border-box;padding:24px;border:1px solid"><img id="scene" width="320" height="192" style="display:block;image-rendering:pixelated"></div>')
            src=f'http://127.0.0.1:{server.server_port}/docs/reviews/stadsvy-alternativ/21b/gata-p4500-m1-normal-huvud-2x.png'
            page.locator('#scene').evaluate('(img,src)=>img.src=src',src);page.locator('#scene').evaluate('(img)=>img.decode()')
            panel=page.locator('#panel').evaluate('(p)=>({outer:p.getBoundingClientRect().width,inner:p.clientWidth-48,scroll:p.scrollWidth,client:p.clientWidth,image:p.querySelector("img").getBoundingClientRect().width})')
            assert panel['outer']==390 and panel['inner']==340 and panel['image']==320 and panel['scroll']==panel['client']
            page.screenshot(path=str(HERE/'firefox-panel390.png'))
            result=dict(browser=b.version,viewport=[1440,390],DPR=1,game_images=32,game_image_css=[320,192],literal_panel=panel,page_errors=errors)
            b.close()
        assert not errors,errors
        return result
    finally:server.shutdown();server.server_close()

if __name__=='__main__':
    result=check();result['firefox']=browser()
    (HERE/'verification.json').write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n');print(json.dumps(result,ensure_ascii=False))
