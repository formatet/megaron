#!/usr/bin/env python3
"""Actual march form/controller/CSS, scripted isolated HTTP; never live accounts."""
import functools,http.server,json,threading,sys,re,subprocess
from pathlib import Path
from playwright.sync_api import sync_playwright,expect
ROOT=Path(__file__).resolve().parents[3];OUT=Path(__file__).resolve().parent
baseline='--baseline' in sys.argv
class Handler(http.server.SimpleHTTPRequestHandler):
 def log_message(self,*args):pass
server=http.server.ThreadingHTTPServer(('127.0.0.1',0),functools.partial(Handler,directory=str(ROOT/'web')))
threading.Thread(target=server.serve_forever,daemon=True).start()
text=subprocess.check_output(['git','-C',str(ROOT),'show','e5daa694:web/static/map.html'],text=True) if baseline else (ROOT/'web/static/map.html').read_text();start=text.index('<div id="march-ctx"');end=text.index('\n<style>',start)
fixture='<meta charset="UTF-8"><link rel="stylesheet" href="/static/megaron.css"><div id="map-root" style="display:none"><canvas id="hex-canvas"></canvas></div><div id="tile-tooltip"></div>'+text[start:end]+''.join(re.findall(r'<style>.*?</style>',text,re.S))
proof=[]
try:
 with sync_playwright() as pw:
  browser=pw.firefox.launch()
  for width in (1000,390):
   for run in (1,2):
    page=browser.new_page(viewport={'width':width,'height':900},device_scale_factor=1);errors=[];posts=[]
    page.on('pageerror',lambda e:errors.append(str(e)))
    page.route('**/fixture',lambda route:route.fulfill(content_type='text/html',body=fixture))
    if baseline:
     original=subprocess.check_output(['git','-C',str(ROOT),'show','e5daa694:web/static/js/megaron/ui/marchctx.js'],text=True)
     page.route('**/ui/marchctx.js',lambda route:route.fulfill(content_type='application/javascript',body=original))
    def api(route):
     req=route.request
     if req.url.endswith('/units'):
      data={'units':[{'id':'unit','type':'spearman','category':'land','status':'garrison','size':100,'settlement_id':'home','display_name':'1st Spearmen of Knossos','deployable':True}], 'expedition_rules':{'min_ticks':4,'max_ticks':365,'default_ticks':10,'area_radius':5}}
     elif '/march-preview?' in req.url:data={'available':True,'arrival_tick':4,'duration_ticks':4}
     elif req.url.endswith('/cities'):data=[{'settlement_id':'home','name':'Knossos','own':True,'knowledge':'known','q':0,'r':0,'terrain':'plains'}, {'settlement_id':'pylos','name':'Pylos','owner':'Nestor','knowledge':'known','q':4,'r':0,'terrain':'plains'},{'settlement_id':'rumour','name':'Rumoured Troy','knowledge':'rumour'},{'settlement_id':'long','name':'Alexandria of the Distant Shore','owner':'Menelaos','knowledge':'known','q':8,'r':0,'terrain':'plains'}]
     elif req.method=='POST':
      posts.append(req.post_data_json);data={'arrival_tick':4,'arrives_at_utc':'2026-10-22T12:00:00Z'}
     else:data=[]
     route.fulfill(status=200,content_type='application/json',body=json.dumps(data))
    page.route('**/api/v1/**',api)
    page.goto(f'http://127.0.0.1:{server.server_port}/fixture')
    page.evaluate('''async()=>{
      const {State}=await import('/static/js/megaron/state.js');
      const mod=await import('/static/js/megaron/ui/marchctx.js');Object.assign(window,mod);
      State.WORLD_ID='fixture';State.CURRENT_TICK=0;State.TICK_SECONDS=60;State.TICK_ANCHOR_MS=Date.now();
      State.provinceData=[{id:'province',settlement_id:'home',name:'Knossos',own:true,q:0,r:0},{id:'pylos-province',settlement_id:'pylos',name:'Pylos',own:false,q:4,r:0}];
      mod.chooseMarchTarget('unit');
    }''')
    expect(page.locator('#march-ctx')).to_be_visible()
    if run==2:page.screenshot(path=str(OUT/(f'firefox-{width}-'+('baseline.png' if baseline else 'select.png'))))
    if not baseline:
     expect(page.locator('#mctx-city')).to_be_enabled()
     choices=page.locator('#mctx-city option').all_text_contents()
     assert 'Knossos' in choices[1] and any('Pylos' in choice for choice in choices) and not any('Troy' in x for x in choices),choices
     page.select_option('#mctx-city','pylos')
     expect(page.locator('#mctx-name')).to_have_text('Pylos')
     expect(page.locator('#mg-0')).to_be_visible()
     expect(page.locator('#mctx-city')).to_have_value('pylos')
     expect(page.locator('#mctx-eta')).to_contain_text('Arrives in')
     if run==2:page.screenshot(path=str(OUT/f'firefox-{width}-selected.png'))
     page.locator('#mctx-send').click()
     expect(page.locator('#mctx-eta')).to_contain_text('Units marching to Pylos')
     assert posts==[{'target_q':4,'target_r':0}],posts
     expect(page.locator('#mctx-send')).to_be_hidden()
     if run==2:page.screenshot(path=str(OUT/f'firefox-{width}-receipt.png'))
     # A fresh map click remains a coordinate order, without using the city select.
     page.evaluate("async()=>{ await window.openMarchCtx({q:7,r:0,name:'Plains (7,0)',terrain:'plains',isSea:false,known:true,isSettlement:false},10,80); }")
     expect(page.locator('#mg-0')).to_be_visible()
     page.locator('#mctx-send').click()
     expect(page.locator('#mctx-eta')).to_contain_text('Units marching')
     assert posts[-1]=={'target_q':7,'target_r':0},posts
     page.evaluate("window.chooseMarchTarget('unit')")
     expect(page.locator('#mctx-city')).to_be_enabled()
     page.select_option('#mctx-city','long')
     expect(page.locator('#mctx-name')).to_have_text('Alexandria of the Distant Shore')
     expect(page.locator('#mg-0')).to_be_visible()
     expect(page.locator('#mctx-eta')).to_contain_text('Arrives in')
     assert page.locator('#march-ctx').evaluate('(el)=>el.scrollWidth<=el.clientWidth')
     if run==2:page.screenshot(path=str(OUT/f'firefox-{width}-long-name.png'))

    assert not errors,errors
    proof.append({'width':width,'run':run,'page_errors':errors,'posts':posts})
    page.close()
  browser.close()
 print(json.dumps(proof,indent=2))
finally:server.shutdown();server.server_close()
