#!/usr/bin/env python3
"""BILD for the Send goods form: real economy.js/send_goods.js/megaron.css and the map.html
drawer chrome in Firefox, a read-only fixture for HTTP (readable names, no uuids). No live game."""
from pathlib import Path
import datetime
import functools,http.server,json,re,threading
from playwright.sync_api import sync_playwright
ROOT=Path(__file__).resolve().parents[1];OUT=ROOT/'docs/reviews/skicka-varor';OUT.mkdir(parents=True,exist_ok=True)
W='world'
class H(http.server.SimpleHTTPRequestHandler):
 def log_message(self,*a):pass
srv=http.server.ThreadingHTTPServer(('127.0.0.1',0),functools.partial(H,directory=str(ROOT/'web')));threading.Thread(target=srv.serve_forever,daemon=True).start()
base=f'http://127.0.0.1:{srv.server_port}'
html=re.sub(r'<script\b[^>]*>.*?</script>','',(ROOT/'web/static/map.html').read_text(),flags=re.S)
html=html.replace('<div class="drawer" id="drawer-economy">','<div class="drawer open" id="drawer-economy">')
boot='''<script type="module">
import {State} from '/static/js/megaron/state.js';
import * as eco from '/static/js/megaron/ui/drawers/economy.js';
State.WORLD_ID='world';State.MY_SETTLEMENT_ID='sA';
State.provinceData=[{id:'pA',settlement_id:'sA',name:'Phaistos',own:true,is_capital:true},{id:'pB',settlement_id:'sB',name:'Kyme',own:true}];
window.eco=eco;window.openDrawer=()=>{};window.ready=true;
</script>'''
html=html.replace('</body>',boot+'</body>')
goodsA=[dict(key='grain',name='Grain',amount=240,shippable=True),dict(key='wine',name='Wine',amount=30,shippable=True),dict(key='timber',name='Timber',amount=75,shippable=True)]
goodsB=[dict(key='wine',name='Wine',amount=5,shippable=True)]
dests=[dict(settlement_id='sB',name='Kyme',owner_name='Me',own=True),dict(settlement_id='sG',name='Gournia',owner_name='Myrine',own=False)]
NOW=datetime.datetime.now(datetime.timezone.utc)
iso=lambda m:(NOW+datetime.timedelta(minutes=m)).strftime('%Y-%m-%dT%H:%M:%SZ')
land=dict(by='land',available=True,travel_ticks=3,travel_seconds=5400,arrives_at=iso(90))
sea=dict(by='sea',available=True,travel_ticks=2,travel_seconds=3600,arrives_at=iso(60),cargo_capacity=120,ship_name='Naia')
def api(route):
 r=route.request;u=r.url.split('/api/v1/worlds/world/')[-1]
 def j(d,s=200):route.fulfill(status=s,content_type='application/json',body=json.dumps(d))
 if r.method=='POST' and u.endswith('/trade'):return j(dict(transport_id='t1',kind='gift' if '"sG"' in (r.post_data or '') else 'transfer',arrival_tick=None,route_version=2,arrives_at=iso(90),travel_ticks=3))
 if u=='provinces/pA/goods':return j(goodsA)
 if u=='provinces/pB/goods':return j(goodsB)
 if u.endswith('/trade/destinations'):return j(dests)
 if '/trade/preview' in u:return j(dict(selected_by='land',options=[land,sea]))
 if u=='trades':return j([dict(id='t1',role='sender',good_key='grain',quantity=50,origin_name='Phaistos',dest_name='Gournia',category='land',arrives_at=iso(90),route_version=2)])
 if u=='standing-orders':return j([dict(id='so1',status='active',from_name='Phaistos',to_name='Kyme',by='land',outbound=[dict(good_key='grain',threshold=200)],**{'return':[]})])
 return j([])
shots=[]
with sync_playwright() as pw:
 b=pw.firefox.launch()
 for w,h in [(1280,900),(390,844)]:
  p=b.new_page(viewport=dict(width=w,height=h),device_scale_factor=1);errs=[]
  p.on('pageerror',lambda e:errs.append(str(e)))
  p.route('https://**',lambda r:r.abort());p.route('**/fixture',lambda r:r.fulfill(content_type='text/html',body=html));p.route('**/api/v1/worlds/**',api)
  p.goto(base+'/fixture');p.wait_for_function('window.ready')
  tag='desktop' if w==1280 else 'mobile'
  p.evaluate('eco.loadEconomyDrawer().then(()=>document.querySelector(\'[data-tab="send"]\').click())')
  p.wait_for_selector('#sg-send');p.wait_for_timeout(300)
  def shot(code,name):
   p.wait_for_timeout(300);f=f'{code}-{name}-{tag}.png';p.screenshot(path=str(OUT/f));shots.append(f)
   assert p.evaluate('document.documentElement.scrollWidth<=innerWidth+1'),'sideways scroll '+f
  # S1 empty form
  shot('S1' if w==1280 else 'S5','empty')
  # S2 foreign chosen + typed
  p.click('[data-act="to"][data-id="sG"]');p.wait_for_selector('.sg-line:has-text("A gift")');p.wait_for_timeout(200)
  p.click('[data-act="all"][data-good="grain"]');shot('S2' if w==1280 else 'S6','foreign')
  # S3 keep checked (own, two routes)
  p.click('[data-act="to"][data-id="sB"]');p.wait_for_selector('[data-act="keep"]');p.wait_for_timeout(200)
  p.check('[data-act="keep"]');p.wait_for_selector('summary');p.click('summary');p.fill('[data-amt="grain"]','200');shot('S3' if w==1280 else 'S7','keep')
  # sent result + fresh row (gift)
  p.uncheck('[data-act="keep"]');p.click('[data-act="to"][data-id="sG"]');p.wait_for_selector('.sg-line:has-text("A gift")');p.wait_for_timeout(200)
  p.click('[data-act="all"][data-good="grain"]');p.click('#sg-send');p.wait_for_selector('#sg-fresh table, #sg-fresh .standing-order-config');shot('S9' if w==1280 else 'S10','sent')
  # S4 on the road
  p.click('[data-tab="road"]');p.wait_for_selector('#standing-order-so1');shot('S4' if w==1280 else 'S8','road')
  assert not errs,errs
 b.close()
print('\n'.join(shots))
