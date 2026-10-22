#!/usr/bin/env python3
"""Actual map modules and explicit local API fixture; never contacts live."""
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
city={'id':'home','settlement_id':'home-settlement','name':'Petras','own':True,'is_capital':True,'q':0,'r':0,'population':1000,'owner':'Minos','culture':'minoan'}
catalogue=[{'type':'farm','hex_bound':True,'costs':{'timber':10},'max_level':3},{'type':'shipyard','costs':{'timber':24,'stone':12},'max_level':3,'upgrade_costs':{'2':{'timber':48,'stone':24,'cedar':4}}}]
quotes=[{'type':'shipyard','current_level':1,'next_level':2,'costs':{'timber':48,'stone':24,'cedar':4},'duration_game_days':24},{'type':'farm','new_site':True,'current_level':0,'next_level':1,'costs':{'timber':10},'duration_game_days':6},{'type':'farm','hex_q':1,'hex_r':0,'current_level':1,'next_level':2,'costs':{'timber':20},'duration_game_days':12},{'type':'farm','hex_q':0,'hex_r':1,'current_level':3,'next_level':None,'at_max_level':True}]
message='Raising Shipyard to level 2 needs cedar (need 4, have 0, 4 short). Petras already has Shipyard level 1.'
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
    def api(route):
     path=route.request.url.split('/api/v1/')[-1];status=200
     if path.endswith('/build'):
      status=422;data={'error':'insufficient_goods','missing':[{'good':'cedar','need':4,'have':0}]}
      if not baseline:data.update(message=message,build_context=quotes[0])
     elif path=='buildings':data=catalogue
     elif path.endswith('/provinces'):data=[city]
     elif path.endswith('/provinces/home'):data={'settlement':{**city,'buildings':[{'type':'shipyard','level':1},{'type':'farm','level':1,'hex_q':1,'hex_r':0},{'type':'farm','level':3,'hex_q':0,'hex_r':1}],'build_quotes':quotes,'resources':{},'labor':{},'devotion':0,'devotion_capacity':0,'temple_offers':[]}}
     elif path.endswith('/placement-options'):data={'hexes':[{'hex_q':1,'hex_r':0,'hex_ordinal':1,'terrain':'plains','building':{'type':'farm','level':1},'goods':[]},{'hex_q':0,'hex_r':1,'hex_ordinal':2,'terrain':'plains','building':{'type':'farm','level':3},'goods':[]}],'buildings':[],'build_quotes':quotes,'valid_hexes_for_building':{'farm':[{'q':-1,'r':0}]},'pool_size':10,'total_gubbar':10}
     elif path.endswith('/placements'):data={'placements':[],'pool_size':10,'total_gubbar':10}
     else:data=[]
     route.fulfill(status=status,content_type='application/json',body=json.dumps(data))
    page.route('**/api/v1/**',api)
    page.add_init_script("window.requestAnimationFrame=()=>0;window.setInterval=()=>0;Date.now=()=>Date.parse('2026-10-22T16:15:00Z');")
    page.goto(f'http://127.0.0.1:{server.server_port}/fixture')
    page.evaluate('''async city=>{await import('/static/js/megaron/main.js');const {State}=await import('/static/js/megaron/state.js');State.WORLD_ID='fixture';State.MY_PLAYER_ID='minos';State.MY_SETTLEMENT_ID='home-settlement';State.provinceData=[city];State.tileData=[];State.CURRENT_TICK=0;State.TICK_SECONDS=60;State.TICK_ANCHOR_MS=Date.now();}''',city)
    def capture(name):
     page.wait_for_load_state('networkidle');page.evaluate('document.fonts.ready');page.evaluate("document.getAnimations().filter(a=>Number.isFinite(a.effect.getComputedTiming().endTime)).forEach(a=>a.finish())")
     page.locator('#city-build-select').evaluate("el=>el.scrollIntoView({block:'center',behavior:'instant'})")
     data=page.screenshot();digest=hashlib.sha256(Image.open(io.BytesIO(data)).tobytes()).hexdigest();key=(width,name)
     if run==1:digests[key]=digest
     else:assert digests[key]==digest,(key,'non-deterministic')
     if run==2:(OUT/f'firefox-{width}-{"baseline-" if baseline else ""}{name}.png').write_bytes(data)
     results.append({'width':width,'run':run,'scene':name,'pixel_sha256':digest})
    page.locator('#trig-city').tap();page.locator('.dtab[data-tab="byggnader"]').tap();expect(page.locator('#city-build-select')).to_be_visible()
    page.locator('#city-build-select').select_option('shipyard');page.wait_for_load_state('networkidle')
    if not baseline:expect(page.locator('#city-build-quote')).to_contain_text('4 cedar')
    capture('shipyard-next')
    page.locator('button[onclick="startBuild()"]').tap();expect(page.locator('#city-build-result')).to_contain_text('cedar')
    if not baseline:expect(page.locator('#city-build-result')).to_contain_text('Petras already has Shipyard level 1')
    capture('shipyard-refusal')
    page.locator('#city-build-select').select_option('farm');expect(page.locator('#city-build-hex')).to_be_visible();page.locator('#city-build-hex').select_option('1,0')
    if not baseline:
     expect(page.locator('#city-build-quote')).to_contain_text('L1 → L2')
     expect(page.locator('#city-build-hex option[value="0,1"]')).to_be_disabled()
     assert 'L4' not in page.locator('#city-build-hex').text_content()
    capture('farm-next-and-max')
    assert not errors,errors;page.close()
  browser.close()
 print(json.dumps(results,indent=2))
finally:server.shutdown();server.server_close()
