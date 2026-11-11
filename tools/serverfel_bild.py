#!/usr/bin/env python3
"""BILD for serverfel: real format.js/inline_result.js/force_card.js/citygrid.js/megaron.css in Firefox.
Fixture only, no live game. E1 rule refusal, E2 technical, E3 logged out, E4 Fit out, E5 worker grid; E6-E10 at 390."""
from pathlib import Path
import functools,http.server,threading
from playwright.sync_api import sync_playwright
ROOT=Path(__file__).resolve().parents[1];OUT=ROOT/'docs/reviews/serverfel';OUT.mkdir(parents=True,exist_ok=True)
class H(http.server.SimpleHTTPRequestHandler):
 def log_message(self,*a):pass
srv=http.server.ThreadingHTTPServer(('127.0.0.1',0),functools.partial(H,directory=str(ROOT/'web')));threading.Thread(target=srv.serve_forever,daemon=True).start()
base=f'http://127.0.0.1:{srv.server_port}'
PAGE='''<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<link rel="stylesheet" href="/static/megaron.css"></head><body><div class="drawer open" style="position:static;width:auto;max-width:420px">
<div class="dsec"><div class="dsec-title">Build</div><div id="host-e1"></div></div>
<div class="dsec"><div class="dsec-title">Send goods</div><div id="host-e2"></div></div>
<div class="dsec"><div class="dsec-title">Recall</div><div id="host-e3"></div></div>
<div class="dsec"><div class="dsec-title">Forces</div><div id="host-e4"></div></div>
<div class="dsec"><div class="dsec-title">Workers</div><div id="host-e5"></div></div></div>
<script type="module">
import {State} from '/static/js/megaron/state.js';
import {showInlineResult} from '/static/js/megaron/ui/inline_result.js';
import {playerError,errorDetail} from '/static/js/megaron/ui/format.js';
import {renderUnitCard} from '/static/js/megaron/ui/force_card.js';
import {goodRowHTML} from '/static/js/megaron/ui/citygrid.js';
const res=s=>({status:s});
const show=(id,status,d)=>{const h=document.getElementById(id);showInlineResult(h,playerError(res(status),d,'Failed'),true,errorDetail(res(status),d));};
show('host-e1',409,{error:'no citizens left in the pool — every citizen is already placed'});
show('host-e2',500,{error:'could not load goods'});
show('host-e3',401,{error:'not authenticated'});
const u={id:'u',type:'galley',display_name:'Dawn Star',category:'naval',status:'laid_up',deployable:false,settlement_id:'home',can_fit_out:false,size:1};
document.getElementById('host-e4').innerHTML=renderUnitCard(u);
document.getElementById('host-e5').innerHTML=goodRowHTML({target_kind:'hex',hex_ordinal:3},{good_key:'grain',cap:4,placed:4,placed_ordinals:[],rate_per_tick:4.8,marginal_yield:1.2});
window.ready=true;
</script></body></html>'''
shots=[]
with sync_playwright() as pw:
 b=pw.firefox.launch()
 for w,h in [(1280,900),(390,844)]:
  tag='desktop' if w==1280 else 'mobile'
  codes=['E1','E2','E3','E4','E5'] if w==1280 else ['E6','E7','E8','E9','E10']
  p=b.new_page(viewport=dict(width=w,height=h),device_scale_factor=1);errs=[]
  p.on('pageerror',lambda e:errs.append(str(e)))
  p.route('https://**',lambda r:r.abort());p.route('**/fixture',lambda r:r.fulfill(content_type='text/html',body=PAGE))
  p.goto(base+'/fixture');p.wait_for_function('window.ready');p.wait_for_timeout(300)
  assert not errs,errs
  assert p.evaluate('document.documentElement.scrollWidth<=innerWidth+1'),'sideways scroll'
  for c,i in zip(codes,range(1,6)):
   sel=f'#host-e{i}'
   el=p.locator(sel).locator('xpath=..')
   f=f'{c}-{tag}.png';el.screenshot(path=str(OUT/f));shots.append(f)
  print(tag,'detail e2:',p.get_attribute('#host-e2 [data-inline-result]','title'),'| e3:',p.get_attribute('#host-e3 [data-inline-result]','data-detail'))
  p.close()
 b.close()
print(shots)
