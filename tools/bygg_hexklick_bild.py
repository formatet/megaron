#!/usr/bin/env python3
"""BILD for building by clicking: real city.js/citygrid.js/build_rows.js/megaron.css and the map.html
drawer chrome in Firefox, a read-only fixture for HTTP (readable names, no uuids). No live game."""
from pathlib import Path
import functools,http.server,json,re,threading
from playwright.sync_api import sync_playwright
ROOT=Path(__file__).resolve().parents[1];OUT=ROOT/'docs/reviews/bygg-hexklick';OUT.mkdir(parents=True,exist_ok=True)
class H(http.server.SimpleHTTPRequestHandler):
 def log_message(self,*a):pass
srv=http.server.ThreadingHTTPServer(('127.0.0.1',0),functools.partial(H,directory=str(ROOT/'web')));threading.Thread(target=srv.serve_forever,daemon=True).start()
base=f'http://127.0.0.1:{srv.server_port}'
html=re.sub(r'<script\b[^>]*>.*?</script>','',(ROOT/'web/static/map.html').read_text(),flags=re.S)
html=html.replace('<div class="drawer" id="drawer-city">','<div class="drawer open" id="drawer-city">')
boot='''<script type="module">
import {State} from '/static/js/megaron/state.js';
import * as city from '/static/js/megaron/ui/drawers/city.js';
State.WORLD_ID='world';State.MY_SETTLEMENT_ID='sA';State.TICK_SECONDS=1800;
State.tileData=[{q:0,r:0,terrain:'plains'}];
State.provinceData=[{id:'pA',settlement_id:'sA',name:'Phaistos',own:true,is_capital:true,q:0,r:0}];
window.city=city;window.openDrawer=()=>{};window.ready=true;
</script>'''
html=html.replace('</body>',boot+'</body>')
RING=[(1,0),(1,-1),(0,-1),(-1,0),(-1,1),(0,1),(2,0),(2,-1),(2,-2),(1,-2),(0,-2),(-1,-1),(-2,0),(-2,1),(-2,2),(-1,2),(0,2),(1,1)]
TERR={0:'plains',1:'plains',2:'forest',3:'hills',4:'plains',5:'forest',6:'plains',7:'plains',8:'hills',9:'forest',10:'plains',11:'plains',12:'hills',13:'plains',14:'forest',15:'plains',16:'plains',17:'sea'}
hexes=[dict(hex_ordinal=i+1,hex_q=q,hex_r=r,terrain=TERR[i],goods=[dict(good_key='grain',placed=1,cap=4,marginal_yield=1.2,rate_per_tick=1.2,placed_ordinals=[1])]) for i,(q,r) in enumerate(RING)]
hexes[1]['building']=dict(type='farm',level=1,upgrade_effect="this hex's grain output ×1.7 → ×2.4")
def quote(t,cur,costs,days,**k):
 nxt=None if k.get('at_max_level') else cur+1
 return dict(type=t,current_level=cur,next_level=nxt,costs=costs,cost_silver=0,duration_game_days=days,**k)
bq=[quote('farm',0,dict(timber=6,stone=4),4,new_site=True),quote('farm',1,dict(timber=12,stone=8),6,hex_q=1,hex_r=-1),
    quote('lumbermill',0,dict(timber=8,stone=2),5,new_site=True),quote('mine',0,dict(timber=5,stone=9),7,new_site=True),quote('stonequarry',0,dict(timber=6,stone=3),5,new_site=True),
    quote('silo',1,dict(timber=8,stone=4),6),quote('barracks',0,dict(timber=14,stone=10),8),quote('stable',0,dict(timber=20,stone=10),9),
    quote('market',0,dict(timber=10,stone=6),6),quote('harbour',3,{},0,at_max_level=True),quote('workshop',0,dict(timber=9,stone=5),5),
    quote('wall',1,dict(timber=20,stone=40),12)]
for g,lvl in [('potnia',1),('velchanos',0),('asterion',0),('sea_daimons_stillness',0)]:
 bq.append(quote('temple',lvl,dict(timber=10,stone=6),8,god_id=g,name='Temple'))
cat=[dict(type=t,costs=dict(timber=6,stone=4),duration_game_days=5,hex_bound=hb) for t,hb in [('farm',True),('lumbermill',True),('mine',True),('stonequarry',True),('silo',False),('barracks',False),('stable',False),('market',False),('harbour',False),('workshop',False),('temple',False)]]
cat.append(dict(type='wall',costs=dict(timber=20,stone=40),duration_game_days=12,wall_ladder=[dict(level=1,name='Palisade',costs=dict(timber=12,stone=4),requires_silo=1,population_cap=5000),dict(level=2,name='Stone Wall',costs=dict(timber=20,stone=40),requires_silo=2,population_cap=10000)]))
valid=dict(farm=[dict(q=0,r=-1,effect="this hex's grain output ×1.7 · space for 4 more workers"),dict(q=-1,r=0,effect="this hex's grain output ×1.7"),dict(q=0,r=1,effect="this hex's grain output ×1.7")],
 lumbermill=[dict(q=0,r=-2,effect='wood output ×1.7'),dict(q=-1,r=-1,effect='wood output ×1.7')],mine=[dict(q=-2,r=0,effect='copper ×1.7')],stonequarry=[dict(q=-2,r=0)])
pd=dict(id='pA',name='Phaistos',population=800,walls=1,silo_level=1,build_queue_max=3,build_queue=[],buildings=[dict(id='b1',type='silo',level=1,name='Granary'),dict(id='b2',type='farm',level=1,name='Farm',hex_q=1,hex_r=-1)],
 build_quotes=bq,can_afford=[dict(type='stable',can_afford=False,next_level=1),dict(type='market',can_afford=True,next_level=1)],resources={},army={},training_units=[],buildable=[])
queued=[]
def api(route):
 r=route.request;u=r.url.split('/api/v1/')[-1]
 def j(d,s=200):route.fulfill(status=s,content_type='application/json',body=json.dumps(d))
 if r.method=='POST' and u.endswith('/build'):
  b=json.loads(r.post_data);queued.append(b);pd['build_queue']=[dict(id='q1',type=b['building_type'],name='Farm',hex_q=b.get('hex_q'),hex_r=b.get('hex_r'),active=False,work_left_game_days=4)]
  return j(dict(duration_game_days=4,active=False),201)
 if u=='buildings':return j(cat)
 if u=='recipes':return j([])
 if u.endswith('/placement-options'):return j(dict(hexes=hexes,buildings=[],total_gubbar=8,pool_size=1,build_quotes=bq,valid_hexes_for_building=valid))
 if u.endswith('/goods'):return j([dict(key='grain',name='Grain',amount=240,idle_citizens=1)])
 if u.endswith('/units'):return j(dict(units=[]))
 if '/provinces/pA' in u:return j(dict(settlement=pd))
 return j([])
shots=[]
with sync_playwright() as pw:
 b=pw.firefox.launch()
 for w,h in [(1280,900),(390,844)]:
  tag='desktop' if w==1280 else 'mobile'
  codes=['H1','H2','H3','H4','H5'] if w==1280 else ['H6','H7','H8','H9','H10']
  del queued[:];pd['build_queue']=[]
  p=b.new_page(viewport=dict(width=w,height=h),device_scale_factor=1);errs=[]
  p.on('pageerror',lambda e:errs.append(str(e)))
  p.route('https://**',lambda r:r.abort());p.route('**/fixture',lambda r:r.fulfill(content_type='text/html',body=html));p.route('**/api/v1/**',api)
  p.goto(base+'/fixture');p.wait_for_function('window.ready')
  p.evaluate('city.loadCityDrawer()');p.wait_for_selector('#city-gubbe-grid svg');p.wait_for_timeout(300)
  def shot(code,name,sel):
   p.wait_for_timeout(300)
   p.evaluate("s=>{const e=document.querySelector(s);e&&e.scrollIntoView({block:'start',behavior:'instant'})}",sel)
   f=f'{code}-{name}-{tag}.png';p.screenshot(path=str(OUT/f));shots.append(f)
   assert p.evaluate('document.documentElement.scrollWidth<=innerWidth+1'),'sideways scroll '+f
   for bt in p.locator('#drawer-city .build-row-btn, #drawer-city .build-pick-bar button, #drawer-city [data-goto-buildings]').all():
    if bt.is_visible():assert bt.bounding_box()['height']>=31,('button under 32px',f,bt.inner_text())
  c=codes
  p.click('.dtab[data-tab="byggnader"]');p.wait_for_selector('.build-row')
  assert p.locator('#city-bld-sec select').count()==0
  shot(c[0],'construct-list','#city-bld-sec .build-list')
  p.evaluate("document.querySelector('.build-row.locked').scrollIntoView({block:'center',behavior:'instant'})")
  shot(c[1],'locked-rows','.build-row.locked')
  p.click('.build-row[data-build-type="farm"] [data-build-act="pick"]');p.wait_for_selector('.build-pick-bar')
  shot(c[2],'pick-mode','.build-pick-bar')
  p.locator('.gubbe-hex.pick-ok').first.dispatch_event('click') if False else p.locator('.gubbe-hex-g:has(.pick-ok)').first.dispatch_event('click')
  p.wait_for_selector('#gubbe-detail .build-row')
  shot(c[3],'hex-in-pick-mode','.build-pick-bar')
  p.click('.build-pick-bar [data-pick-cancel]');p.wait_for_selector('.build-pick-bar',state='detached')
  p.locator('.gubbe-hex-g[data-ordinal="3"]').dispatch_event('click');p.wait_for_selector('#gubbe-detail .dsec-title:has-text("Build here")')
  shot(c[4],'build-here','#gubbe-detail')
  p.click('#gubbe-detail [data-build-act="build-hex"]');p.wait_for_selector('.gubbe-built')
  assert queued==[dict(building_type='farm',hex_q=0,hex_r=-1)],queued
  assert not errs,errs
  p.close()
 b.close()
print('\n'.join(shots))
