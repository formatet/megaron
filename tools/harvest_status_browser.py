#!/usr/bin/env python3
"""Actual city drawer + renderer + CSS; explicit read-only Mycenae HTTP fixture.
The before fixture omits the blessing in the current UI; it is not archived UI.
No live-game acceptance claim.
"""
from pathlib import Path
import functools,http.server,json,re,threading,hashlib
from playwright.sync_api import sync_playwright,expect
ROOT=Path(__file__).resolve().parents[1];OUT=ROOT/'docs/reviews/skordegava-syns/browser';OUT.mkdir(parents=True,exist_ok=True)
WORLD='11111111-1111-1111-1111-111111111111';PROV='22222222-2222-2222-2222-222222222222';SETT='33333333-3333-3333-3333-333333333333'
provinces=[dict(id=PROV,settlement_id=SETT,name='Mycenae',owner='Agamemnon',own=True,is_capital=True,q=0,r=0,culture='achaean',walls=1)]
tiles=[dict(q=q,r=r,terrain='plains',tier='live',visibility='live',deposits=[]) for q in range(-10,11) for r in range(-8,9)]
# Same values as owning code, extracted rather than a second tuning constant.
owner=(ROOT/'server/internal/economy/harvest_blessing.go').read_text()
bonus=float(re.search(r'HarvestBlessingYieldBonus\s*=\s*([\d.]+)',owner)[1])
goods_source=(ROOT/'server/internal/economy/goods.go').read_text()
food=re.search(r'var FoodGoods = \[\]string\{([^}]+)',goods_source)[1]
keys=dict(re.findall(r'(Good\w+)\s*=\s*"([a-z]+)"',goods_source))
goods=[keys[g.strip()] for g in food.split(',') if g.strip()!='GoodFish']
blessing=dict(until_tick=112,remaining_ticks=12,yield_bonus=bonus,goods=goods)
sett=dict(id=SETT,name='Mycenae',population=1000,labor_pool=10,army={},walls=1,buildings=[],build_queue=[],can_afford=[],can_recruit=[],resources={},available_prayers=[],food_gubbar_required=7,food_gubbar_placed=7,food_self_sufficient=True,grain_prod_rate=12,grain_consum_rate=5,food=dict(coverage_ticks=12,food_net_per_tick=7,growth_state='growing'))
(OUT/'fixture.json').write_text(json.dumps(dict(world=WORLD,province=PROV,settlement=dict(sett,harvest_blessing=blessing),placement_options=dict(hexes=[],buildings=[],total_gubbar=10,pool_size=3)),indent=2)+'\n')
class Handler(http.server.SimpleHTTPRequestHandler):
 def log_message(self,*args):pass
server=http.server.ThreadingHTTPServer(('127.0.0.1',0),functools.partial(Handler,directory=str(ROOT/'web')));threading.Thread(target=server.serve_forever,daemon=True).start();base=f'http://127.0.0.1:{server.server_port}'
html=re.sub(r'<script\b[^>]*>.*?</script>','',(ROOT/'web/static/map.html').read_text(),flags=re.S)
results=[]
try:
 with sync_playwright() as pw:
  browser=pw.firefox.launch()
  for width,height in [(1280,900),(390,844)]:
   for mode in ['before','active','end','active-repeat']:
    page=browser.new_page(viewport={'width':width,'height':height},device_scale_factor=1);errors=[];unexpected=[]
    page.on('pageerror',lambda e:errors.append(str(e)))
    page.on('console',lambda m:errors.append(m.text) if m.type=='error' else None)
    page.route('https://**',lambda r:r.abort())
    page.route('**/fixture',lambda r:r.fulfill(content_type='text/html',body=html))
    def api(route):
     req=route.request;tail=req.url.split('/api/v1/worlds/'+WORLD+'/')[-1]
     if req.method!='GET':unexpected.append(tail);route.fulfill(status=500,body='{}');return
     if req.url.endswith('/api/v1/recipes') or req.url.endswith('/api/v1/buildings'):data=[]
     elif tail=='provinces/'+PROV+'/actions':data=[]
     elif tail=='provinces/'+PROV:data={'settlement':dict(sett,harvest_blessing=None if mode in ['before','end'] else blessing)}
     elif tail=='provinces/'+PROV+'/goods':data=[dict(key='grain',name='Grain',amount=100,rate_per_tick=7,idle_citizens=300)]
     elif tail=='provinces/'+PROV+'/placement-options':data=dict(hexes=[],buildings=[],total_gubbar=10,pool_size=3)
     elif tail=='map':data=tiles
     elif tail=='provinces':data=provinces
     elif tail=='units':data={'units':[]}
     elif tail in ['marches','messengers','trades','rural-projections','foreign-units','settlements/placement-roster','storms']:data=[]
     else:unexpected.append(tail);route.fulfill(status=500,body='{}');return
     route.fulfill(content_type='application/json',body=json.dumps(data))
    page.route('**/api/v1/**',api)
    page.goto(base+'/fixture',wait_until='domcontentloaded')
    page.evaluate('''async([world,prov,sett,provinces])=>{
     localStorage.setItem('poleia_token','harvest-visual-fixture');window.MusicPlayer={update(){}};
     window.State=(await import('/static/js/megaron/state.js')).State;
     Object.assign(State,{WORLD_ID:world,MY_SETTLEMENT_ID:sett,MY_PLAYER_ID:'Agamemnon',CURRENT_TICK:100,WORLD_STATE:'active',provinceData:provinces,cityViewID:prov});
     document.getElementById('gt-wanax').textContent='Wanax Agamemnon';
     document.getElementById('net-status').textContent='Mycenae';
     window.map=await import('/static/js/megaron/render/map.js');map.initMap();
     window.city=await import('/static/js/megaron/ui/drawers/city.js');
     document.getElementById('drawer-city').classList.add('open');await city.loadCityDrawer();
    }''',[WORLD,PROV,SETT,provinces])
    expect(page.locator('#city-food-sec')).to_contain_text('Food lasts')
    expect(page.locator('#city-drawer-title')).to_contain_text('Mycenae')
    if mode in ['active','active-repeat']:
     line=page.locator('.harvest-blessing');expect(line).to_contain_text(f'+{bonus*100:g}% '+', '.join(goods)+' yield · 12 days left');line.scroll_into_view_if_needed()
     bounds=line.bounding_box();assert bounds['x']>=0 and bounds['x']+bounds['width']<=width+1,(width,bounds)
     assert line.evaluate('(el)=>el.scrollWidth<=el.clientWidth'),'harvest text overflows'
     assert not re.search(r'\bticks?\b|\bfish\b',line.inner_text())
    else:assert page.locator('.harvest-blessing').count()==0,'row must be absent'
    page.locator('#city-food-sec').scroll_into_view_if_needed()
    page.wait_for_function('map.markrasterStats.pending===0 && map.markrasterStats.chunks>0')
    page.wait_for_timeout(350)
    page.screenshot(path=str(OUT/f'firefox-{width}-{mode}.png'))
    results.append(dict(width=width,mode=mode,food=page.locator('#city-food-sec').inner_text(),errors=errors,unexpected=unexpected))
    assert not errors and not unexpected,results[-1]
    page.close()
  browser.close()
finally:server.shutdown()
(OUT/'results.json').write_text(json.dumps(dict(browser='Firefox',fixture='explicit HTTP + State/bootstrap; actual CSS/drawer/map renderer, no gameplay claims',source_sha256={p:hashlib.sha256((ROOT/p).read_bytes()).hexdigest() for p in ['web/static/js/megaron/ui/drawers/city_production.js','web/static/js/megaron/ui/drawers/city.js','web/static/megaron.css']},results=results),indent=2)+'\n')
print('Firefox 1280/390 before, active, end, repeat: PASS')

# Inactive keeps the locked baseline pixels; repeated active rendering is stable.
from PIL import Image,ImageChops
comparisons=[]
for width in [1280,390]:
 for a,b in [('active','active-repeat'),('before','end')]:
  left=Image.open(OUT/f'firefox-{width}-{a}.png').convert('RGB')
  right=Image.open(OUT/f'firefox-{width}-{b}.png').convert('RGB')
  diff=ImageChops.difference(left,right)
  pixels=sum(pixel!=(0,0,0) for pixel in diff.get_flattened_data())
  comparisons.append(dict(width=width,a=a,b=b,different_pixels=pixels,bbox=diff.getbbox()))
  assert pixels==0,comparisons[-1]
(OUT/'pixeldiff.json').write_text(json.dumps(comparisons,indent=2)+'\n')
