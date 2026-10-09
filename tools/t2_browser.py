#!/usr/bin/env python3
"""T2 browser proof: real archived DB payloads, actual dispatch/Codex/status
modules and CSS. Only preferences HTTP is an explicit read-only fixture; this
is a rendering rig, not a claim of a live game lifecycle (Go proves that).
Firefox first, then Chromium/WebKit, desktop and 390x844.
"""
import argparse
import functools
import http.server
import json
from pathlib import Path
import re
import threading
from playwright.sync_api import sync_playwright,expect

ROOT=Path(__file__).resolve().parents[1]
p=argparse.ArgumentParser(description=__doc__);p.add_argument('--output-dir',type=Path,default=ROOT/'docs/reviews/t2-lopare/browser');p.add_argument('--browsers',nargs='+',choices=['firefox','chromium','webkit'],default=['firefox','chromium','webkit']);a=p.parse_args();a.output_dir.mkdir(parents=True,exist_ok=True)
class Handler(http.server.SimpleHTTPRequestHandler):
    def log_message(self,*args):pass
server=http.server.ThreadingHTTPServer(('127.0.0.1',0),functools.partial(Handler,directory=str(ROOT/'web')))
threading.Thread(target=server.serve_forever,daemon=True).start()
base=f'http://127.0.0.1:{server.server_port}'
html=re.sub(r'<script\b[^>]*>.*?</script>','',(ROOT/'web/static/map.html').read_text(),flags=re.S)
fixtures=ROOT/'web/static/js/megaron/ui/testdata'
payloads={name:json.loads((fixtures/file).read_text()) for name,file in [('letter','messenger_lost_at_sea.json'),('trade','messenger_lost_trade.json'),('order','messenger_lost_order.json'),('rescue','messenger_rescued_at_sea.json')]}
results=[]
try:
 with sync_playwright() as pw:
  for engine in a.browsers:
   browser=getattr(pw,engine).launch()
   try:
    for mode,width,height in [('desktop',1280,900),('mobile',390,844)]:
     page=browser.new_page(viewport={'width':width,'height':height});errors=[];calls=[]
     page.on('pageerror',lambda e:errors.append(str(e)))
     page.route('**/fixture',lambda r:r.fulfill(content_type='text/html',body=html))
     def api(route):
      req=route.request
      assert req.method=='GET' and req.url.endswith('/notification-preferences'),req.url
      assert req.headers.get('authorization')=='Bearer t2-read-fixture'
      calls.append(req.url);route.fulfill(content_type='application/json',body='{"muted_kinds":[]}')
     page.route('**/api/**',api)
     page.goto(base+'/fixture',wait_until='domcontentloaded')
     page.evaluate('''async()=>{
      localStorage.setItem('poleia_token','t2-read-fixture');
      window.t2dispatch=await import('/static/js/megaron/ui/dispatch_window.js');
      window.closeDispatchWindow=t2dispatch.closeDispatchWindow;
      window.cx=await import('/static/js/megaron/ui/codex.js');cx.initCodex();
     }''')
     page.wait_for_function("cx.codexArticleForKind('MessengerLostAtSea') === 'runner-lost-at-sea'")
     for label in ['letter','trade','order']:
      body=payloads[label]
      page.evaluate('(body)=>t2dispatch.openDispatchWindow("MessengerLostAtSea",body,"archived game outcome")',body)
      expect(page.locator('#dispatch-window-overlay')).to_be_visible()
      text=page.locator('.dw-envelope').inner_text()
      assert body['envelope']['message_text'] in text
      assert 'From you, at Mycenae, to ' in text
      target = 'your Bronze Guard at (9, 4)' if label=='order' else 'Wanax Oledoledoff at Tiryns'
      assert target in text
      assert page.locator('.dw-text').inner_text().startswith('Your runner to '+target+' was lost at sea')
      assert 'from you' not in page.locator('.dw-text').inner_text()
      assert not any(word in page.locator('.dw-text').inner_text() for word in ['Passage-','private-login-','Changed Guard'])
      assert f"Sent on day {body['envelope']['sent_tick']}" in text
      assert body['envelope']['sent_at'] not in text
      for key in ['trade_offer','order_payload']:
       if body['envelope'].get(key) is not None:
        for value in ['bronze','145.7'] if key=='trade_offer' else ['march','hold_to_last_man']:
         assert value in text,(label,key,value)
      assert page.locator('.dw-envelope script').count()==0
      bounds=page.locator('#dispatch-window-overlay .dispatch-window').bounding_box()
      assert bounds['x']>=-1 and bounds['x']+bounds['width']<=width+1,bounds
      assert bounds['y']+bounds['height']<=height+1,bounds
      expect(page.locator('#dw-codex-btn')).to_be_visible()
      if label=='letter':
       page.locator('#dispatch-window-overlay .dw-body').evaluate('(el)=>el.scrollTop=0')
       page.screenshot(path=str(a.output_dir/f'{engine}-{mode}-loss-top.png'))
      if label=='order':
       page.locator('#dw-mute-chk').scroll_into_view_if_needed()
       expect(page.locator('#dw-mute-chk')).to_be_visible()
       page.screenshot(path=str(a.output_dir/f'{engine}-{mode}-loss-bottom.png'))
      page.evaluate('t2dispatch.closeDispatchWindow()')
     page.evaluate('''(body)=>{t2dispatch.openDispatchWindow("MessengerRescuedAtSea",body,"");document.getElementById('dispatch-window-overlay').scrollTop=0;document.getElementById('dw-body').scrollTop=0;window.scrollTo(0,0)}''',payloads['rescue'])
     expect(page.locator('.dw-text')).to_contain_text(f"Home on day {payloads['rescue']['home_tick']}.")
     expect(page.locator('.dw-text')).to_contain_text('Sacred Dolphin')
     expect(page.locator('.dw-text')).to_contain_text('ashore at Tiryns')
     expect(page.locator('.dw-text')).to_have_text(f"Home on day {payloads['rescue']['home_tick']}. Your runner was rescued at sea by the Sacred Dolphin and put ashore at Tiryns, then went ashore at Mycenae.")
     assert 'Passage-' not in page.locator('.dw-text').inner_text()
     page.wait_for_timeout(150)
     page.evaluate("document.getElementById('dispatch-window-overlay').scrollTop=0;document.getElementById('dw-body').scrollTop=0;window.scrollTo(0,0)")
     page.wait_for_timeout(50)
     assert page.locator('#dispatch-window-overlay .dw-header').bounding_box()['y']>=0
     page.screenshot(path=str(a.output_dir/f'{engine}-{mode}-rescue.png'))
     page.locator('#dw-codex-btn').click()
     expect(page.locator('#codex-panel')).to_be_visible()
     expect(page.locator('#codex-body')).to_contain_text('next port')
     expect(page.locator('#codex-body')).to_contain_text('no word')
     page.screenshot(path=str(a.output_dir/f'{engine}-{mode}-codex.png'))
     page.evaluate('cx.closeCodex()')
     # Same module used by the actual Diplomacy outbox, with deliberately
     # hostile hidden fields to prove no visible carrier/port/ETA hint.
     page.evaluate('''async()=>{
      const {sentStatusHTML}=await import('/static/js/megaron/ui/runner_status.js');
      document.getElementById('dw-body').innerHTML=sentStatusHTML({status:'outbound',passage_status:'unknown',carrier_name:'SECRET SHIP',passage_port:'SECRET PORT',arrives_at:'2099-01-01T00:00:00Z'});
      document.getElementById('dispatch-window-overlay').classList.add('open');
     }''')
     expect(page.locator('#dw-body')).to_contain_text('no word')
     assert not any(secret in page.locator('#dw-body').inner_text() for secret in ['SECRET','arrives','aboard','sealed'])
     page.screenshot(path=str(a.output_dir/f'{engine}-{mode}-no-word.png'))
     assert not errors,errors
     result={'browser':engine,'viewport':{'width':width,'height':height},'payloads':list(payloads),'full_contents':True,'scroll_controls_reachable':True,'unknown_position':True,'codex':True,'page_errors':errors,'read_only_preference_requests':len(calls)}
     results.append(result);print(engine+' '+mode+': PASS',flush=True);page.close()
   finally:browser.close()
 (a.output_dir/'results.json').write_text(json.dumps(results,indent=2)+'\n')
finally:server.shutdown()
