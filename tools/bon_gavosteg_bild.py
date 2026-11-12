#!/usr/bin/env python3
"""BILD for the prayer gift steps: real kult.js/kult_offer.js/megaron.css and the map.html drawer chrome
in Firefox, a read-only fixture for HTTP (readable names, no uuids). No live game."""
from pathlib import Path
import functools,http.server,json,re,threading
from playwright.sync_api import sync_playwright
ROOT=Path(__file__).resolve().parents[1];OUT=ROOT/'docs/reviews/bon-gavosteg';OUT.mkdir(parents=True,exist_ok=True)
class H(http.server.SimpleHTTPRequestHandler):
 def log_message(self,*a):pass
srv=http.server.ThreadingHTTPServer(('127.0.0.1',0),functools.partial(H,directory=str(ROOT/'web')));threading.Thread(target=srv.serve_forever,daemon=True).start()
base=f'http://127.0.0.1:{srv.server_port}'
html=re.sub(r'<script\b[^>]*>.*?</script>','',(ROOT/'web/static/map.html').read_text(),flags=re.S)
html=html.replace('<div class="drawer" id="drawer-kult">','<div class="drawer open" id="drawer-kult">')
boot='''<script type="module">
import {State} from '/static/js/megaron/state.js';
import * as kult from '/static/js/megaron/ui/drawers/kult.js';
State.WORLD_ID='world';State.MY_SETTLEMENT_ID='sK';
Object.assign(window,{okRite:kult.okRite,bringOffering:kult.bringOffering,cancelOffering:kult.cancelOffering,openDrawer:()=>{},kult,ready:true});
</script>'''
html=html.replace('</body>',boot+'</body>')
common=dict(god_id='potnia',god='Potnia',temple='Temple of Potnia',min_kharis=5,affordable=True,effect='Fixture.',rest_remaining_game_days=0)
walls=lambda sid,pid,city:dict(common,settlement_id=sid,province_id=pid,city_name=city,id='minoan_potnia_walls',name='Mistress of the Walls',offering=dict(grain=8,timber=4),base_duration_game_days=6,effect_type='garrison_defense')
vision=dict(common,settlement_id='sK',province_id='pK',city_name='Knossos',id='minoan_potnia_vision_v2',name="Potnia's Vision",offering=dict(oil=3,silver=2),base_duration_game_days=0,effect_type='oracle_reveal_deposits')
hull=dict(common,god_id='sea_daimons_stillness',god='Sea Daimons',settlement_id='sK',province_id='pK',city_name='Knossos',id='minoan_stillness_hull',name='Stout Hulls',offering=dict(fish=8,oil=4),base_duration_game_days=6,effect_type='blessed_hull',
 scope_cost_factors=dict(unit=1,city=2,realm=4,permanent=20),targets=dict(unit=[dict(id='u1',name='The Naia',where='in Knossos')],city=dict(name='Knossos',count=1),realm=dict(count=2)))
view=dict(kharis={},temples=[],unbuilt_temples=[],prayers=[walls('sK','pK','Knossos'),walls('sP','pP','Phaistos'),vision,hull])
stock=dict(pK=dict(grain=500,timber=300,oil=300,silver=100,fish=400),pP=dict(grain=20,timber=10,oil=5,fish=9))
def api(route):
 r=route.request;u=r.url.split('/api/v1/worlds/world/')[-1]
 def j(d):route.fulfill(status=200,content_type='application/json',body=json.dumps(d))
 if u=='cult':return j(view)
 m=re.match(r'provinces/(p\w)/goods',u)
 if m:return j([dict(key=k,name=k,amount=v) for k,v in stock[m.group(1)].items()])
 m=re.match(r'provinces/(p\w)$',u)
 if m:return j(dict(settlement=dict(food=dict(consumption_per_tick=20))))
 return j([])
shots=[]
with sync_playwright() as pw:
 b=pw.firefox.launch()
 for w,h in [(1280,900),(390,844)]:
  tag='desktop' if w==1280 else 'mobile'
  p=b.new_page(viewport=dict(width=w,height=h),device_scale_factor=1);errs=[]
  p.on('pageerror',lambda e:errs.append(str(e)))
  p.route('https://**',lambda r:r.abort());p.route('**/fixture',lambda r:r.fulfill(content_type='text/html',body=html));p.route('**/api/v1/worlds/**',api)
  p.goto(base+'/fixture');p.wait_for_function('window.ready')
  p.evaluate("kult.loadKultDrawer()");p.wait_for_selector('[data-cult-panel="prayers"]',state='attached')
  p.evaluate("document.querySelector('[data-cult-tab=prayers]').click()")
  def card(pid):return p.locator(f'[data-cult-prayer="{pid}"]')
  def shot(code,name,pid):
   p.wait_for_timeout(300);el=card(pid);el.scroll_into_view_if_needed();f=f'{code}-{name}-{tag}.png';el.screenshot(path=str(OUT/f));shots.append(f)
   assert p.evaluate('document.documentElement.scrollWidth<=innerWidth+1'),'sideways scroll '+f
   for rad in p.locator(f'[data-cult-prayer="{pid}"] .offer-step').all():
    assert rad.bounding_box()['height']>=43,rad.bounding_box()
  def open_offer(pid):
   card(pid).locator('button:has-text("Offer")').first.click();p.wait_for_selector(f'#offer-row-{pid} .offer-gifts')
  # a: all payable
  open_offer('sK__minoan_potnia_walls');shot('A','all-steps-payable','sK__minoan_potnia_walls')
  # e: other goods open
  p.click('#offer-row-sK__minoan_potnia_walls summary');shot('E','other-goods-open','sK__minoan_potnia_walls')
  p.evaluate("kult.cancelOffering('sK__minoan_potnia_walls')")
  # b: two unpayable
  open_offer('sP__minoan_potnia_walls');shot('B','two-steps-unpayable','sP__minoan_potnia_walls')
  # c: instant
  open_offer('sK__minoan_potnia_vision_v2');shot('C','instant-prayer','sK__minoan_potnia_vision_v2')
  # d: for ever
  open_offer('sK__minoan_stillness_hull')
  p.click('input[name="scope-sK__minoan_stillness_hull"][value="city"]')
  p.click('#offer-row-sK__minoan_stillness_hull .offer-step-radio[value="3"]');shot('D1','hull-city-three-times','sK__minoan_stillness_hull')
  p.click('input[name="scope-sK__minoan_stillness_hull"][value="permanent"]');shot('D2','for-ever','sK__minoan_stillness_hull')
  assert not errs,errs
 b.close()
print('\n'.join(shots))
