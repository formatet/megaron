#!/usr/bin/env python3
"""Actual map modules/HTML, explicit local HTTP fixture. No live writes."""
from pathlib import Path
import functools,http.server,threading,re,json,subprocess,sys,hashlib,io
from PIL import Image
from playwright.sync_api import sync_playwright,expect
ROOT=Path(__file__).resolve().parents[3];OUT=Path(__file__).resolve().parent
baseline='--baseline' in sys.argv
class Handler(http.server.SimpleHTTPRequestHandler):
 def log_message(self,*args):pass
server=http.server.ThreadingHTTPServer(('127.0.0.1',0),functools.partial(Handler,directory=str(ROOT/'web')))
threading.Thread(target=server.serve_forever,daemon=True).start()
def source(path):return subprocess.check_output(['git','-C',str(ROOT),'show','ea97b332:'+path],text=True) if baseline else (ROOT/path).read_text()
html=re.sub(r'<script\b[^>]*>.*?</script>','',source('web/static/map.html'),flags=re.S)
results=[];digests={}
try:
 with sync_playwright() as pw:
  browser=pw.firefox.launch()
  for width in (390,1000):
   for run in (1,2):
    page=browser.new_page(viewport={'width':width,'height':844},has_touch=True,device_scale_factor=1);errors=[]
    page.on('pageerror',lambda e:errors.append(str(e)))
    page.route('**/fixture',lambda r:r.fulfill(content_type='text/html',body=html))
    def asset(route):
     path='web/static/'+route.request.url.split('/static/')[-1];body=source(path)
     if path.endswith('/main.js'):body=body.split('(async function start() {')[0]
     route.fulfill(content_type='text/css' if path.endswith('.css') else 'application/javascript',body=body)
    page.route('**/static/js/**',asset);page.route('**/static/megaron.css',asset)
    city={'id':'home','settlement_id':'home-settlement','name':'Knossos','own':True,'is_capital':True,'q':0,'r':0,'population':1000,'owner':'Minos','culture':'minoan'}
    tiles=[{'q':q,'r':r,'terrain':'plains','tier':'live'} for q in range(-4,8) for r in range(-4,6)]
    staffing=0;level=1;population=1000
    def api(route):
     global staffing, level, population
     path=route.request.url.split('/api/v1/')[-1]
     if path.endswith('/placements') and route.request.method=='POST':
      if staffing>=level*2 or staffing>=population//100:route.fulfill(status=409,content_type='application/json',body='{"error":"No idle citizen available"}');return
      staffing+=1;data={'ordinal':staffing}
     elif '/placements/' in path and route.request.method=='DELETE':staffing-=1;data={'ok':True}
     elif path.endswith('/provinces'):data=[city]
     elif path.endswith('/map'):data=tiles
     elif path.endswith('/provinces/home'):data={'settlement':{**city,'buildings':[{'type':'temple','level':1}],'resources':{},'labor':{},'devotion':.15,'devotion_capacity':.15,'temple_staffed':staffing,'temple_capacity':level*2,'kharis_city_count':1,'kharis_base_decay':.1,'kharis_city_decay':0,'kharis_daily_decay':.1,'kharis_devotion_idle':staffing<level*2,'kharis_net_per_tick':staffing*1.05-.1,'kharis_net_known':True,'temple_offers':[{'settlement_id':'home-settlement','name':'Knossos','required_goods':{'grain':.25},'alternatives':{'grain':.25},'stock':{'grain':100},'offering':{'grain':.25},'fed':True}],'available_prayers':[{'name':'Harvest blessing','god':'Demeter','min_kharis':5,'offering':{'grain':1},'available':True}]}}
     elif path.endswith('/settlements/home-settlement'):data={'divine_mood':'Indifferent','cult_level':'enkel','kharis':20}
     elif path.endswith('/placement-options'):data={'hexes':[],'buildings':[] if baseline else [{'building_type':'temple','level':level,'goods':[{'good_key':'cult','staffing_only':True,'cap':2*level,'placed':staffing,'placed_ordinals':list(range(1,staffing+1)),'rate_per_tick':0,'marginal_yield':0}]}],'pool_size':population//100-staffing,'total_gubbar':population//100}
     elif path.endswith('/placements'):data={'placements':[{'gubbe_ordinal':n,'good_key':'cult','target_kind':'building','building_type':'temple'} for n in range(1,staffing+1)],'pool_size':population//100-staffing,'total_gubbar':population//100}
     else:data=[]
     route.fulfill(content_type='application/json',body=json.dumps(data))
    page.route('**/api/v1/**',api)
    page.add_init_script("window.requestAnimationFrame=()=>0;window.setInterval=()=>0;Date.now=()=>Date.parse('2026-10-22T16:15:00Z');")
    page.goto(f'http://127.0.0.1:{server.server_port}/fixture')
    page.evaluate('''async args=>{await import('/static/js/megaron/main.js');const {State}=await import('/static/js/megaron/state.js');State.WORLD_ID='fixture';State.MY_PLAYER_ID='minos';State.MY_SETTLEMENT_ID='home-settlement';State.provinceData=[args.city];State.tileData=args.tiles;State.CURRENT_TICK=0;State.TICK_SECONDS=60;State.TICK_ANCHOR_MS=Date.now();const map=await import('/static/js/megaron/render/map.js');map.initMap();await map.loadMap();State.camera.x=150;State.camera.y=160;State.animFrame=63;for(let i=0;i<40;i++){State.dirty=true;map.render();}}''',{'city':city,'tiles':tiles})
    def settle():
     page.wait_for_load_state('networkidle');page.evaluate('document.fonts.ready');page.evaluate("document.getAnimations().filter(a=>Number.isFinite(a.effect.getComputedTiming().endTime)).forEach(a=>a.finish())")
    def capture(name):
     settle()
     if name.startswith('city-'):
      page.locator('.gubbe-good-row[data-good="cult"]').evaluate("el=>el.scrollIntoView({block:'center',behavior:'instant'})")
     data=page.screenshot();digest=hashlib.sha256(Image.open(io.BytesIO(data)).tobytes()).hexdigest();key=(width,name)
     (OUT/f'debug-{width}-{name}-run{run}.png').write_bytes(data)
     if run==1:digests[key]=digest
     else:assert digests[key]==digest,(key,'non-deterministic')
     if run==2:(OUT/f'firefox-{width}-{"baseline-" if baseline else ""}{name}.png').write_bytes(data)
     results.append({'width':width,'run':run,'scene':name,'pixel_sha256':digest})
    page.locator('#trig-city').tap();expect(page.locator('#city-scene')).to_be_visible();settle()
    if baseline:capture('city')
    else:
     page.locator('[data-target="city"]').first.tap();expect(page.locator('.gubbe-good-row[data-good="cult"]')).to_be_visible();capture('city-empty')
     row=lambda:page.locator('.gubbe-good-row[data-good="cult"]')
     for n in (1,2):
      row().locator('[data-verb="place1"]').tap();expect(row()).to_have_attribute('data-cap','2');expect(row()).to_contain_text(f'{n} temple workers');capture(f'city-{n}-workers')
     expect(row().locator('[data-verb="place1"]')).to_be_disabled()
     # The real write is refused even if bypassing a disabled client button.
     assert page.evaluate("async()=>{const r=await fetch('/api/v1/worlds/fixture/provinces/home/placements',{method:'POST'});return r.status}")==409
     row().locator('[data-verb="unplace1"]').tap();expect(row()).to_contain_text('1 temple workers');capture('city-worker-freed')
     level=2;page.evaluate("async()=>{const {renderGubbeGrid}=await import('/static/js/megaron/ui/citygrid.js');await renderGubbeGrid(document.querySelector('#city-gubbe-grid'),'home',0,0)}")
     page.locator('[data-target="city"]').first.tap();expect(row()).to_have_attribute('data-cap','4');capture('city-l2')
     population=100;page.evaluate("async()=>{const {renderGubbeGrid}=await import('/static/js/megaron/ui/citygrid.js');await renderGubbeGrid(document.querySelector('#city-gubbe-grid'),'home',0,0)}")
     page.locator('[data-target="city"]').first.tap();row().locator('[data-verb="place1"]').tap();expect(page.locator('#gubbe-detail')).to_contain_text('No idle citizen available');capture('city-no-idle')
    page.evaluate("closeDrawer('city')");page.locator('#trig-kult').tap();expect(page.locator('#kult-body .dsec').first).to_be_visible();capture('kult')
    if not baseline:
     expect(page.locator('#kult-body')).to_contain_text('Temple workers here');expect(page.locator('#kult-body')).to_contain_text('1/4')
    page.evaluate("closeDrawer('kult')")
    assert not errors,errors
    page.close()
  browser.close()
 print(json.dumps(results,indent=2))
finally:server.shutdown();server.server_close()
