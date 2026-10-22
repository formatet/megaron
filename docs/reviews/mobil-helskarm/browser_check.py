#!/usr/bin/env python3
"""Real map DOM, modules and panel rendering; isolated explicit HTTP fixture; touch taps."""
import functools,http.server,json,re,threading,subprocess,sys,hashlib
from PIL import Image
import io
from pathlib import Path
from playwright.sync_api import sync_playwright,expect
ROOT=Path(__file__).resolve().parents[3];OUT=Path(__file__).resolve().parent
baseline='--baseline' in sys.argv
class Handler(http.server.SimpleHTTPRequestHandler):
 def log_message(self,*args):pass
server=http.server.ThreadingHTTPServer(('127.0.0.1',0),functools.partial(Handler,directory=str(ROOT/'web')))
threading.Thread(target=server.serve_forever,daemon=True).start()
def source(path):
 return subprocess.check_output(['git','-C',str(ROOT),'show','d6b0032c:'+path],text=True) if baseline else (ROOT/path).read_text()
html=re.sub(r'<script\b[^>]*>.*?</script>','',source('web/static/map.html'),flags=re.S)
proof=[];pixels={}
try:
 with sync_playwright() as pw:
  browser=pw.firefox.launch()
  for width in (390,1000):
   for run in (1,2):
    page=browser.new_page(viewport={'width':width,'height':844},device_scale_factor=1,has_touch=True);errors=[]
    page.on('pageerror',lambda e:errors.append(str(e)));page.set_default_timeout(8000)
    page.route('**/fixture',lambda r:r.fulfill(content_type='text/html',body=html))
    if baseline:
     def original(route):
      path=route.request.url.split('/static/')[-1]
      route.fulfill(content_type='text/css' if path.endswith('.css') else 'application/javascript',body=source('web/static/'+path).split('(async function start() {')[0] if path.endswith('/main.js') else source('web/static/'+path))
     page.route('**/static/megaron.css',original);page.route('**/static/js/**',original)
    else:
     page.route('**/js/megaron/main.js',lambda r:r.fulfill(content_type='application/javascript',body=source('web/static/js/megaron/main.js').split('(async function start() {')[0]))
    city={'id':'home','settlement_id':'home-settlement','name':'Knossos','own':True,'is_capital':True,'q':0,'r':0,'population':1000,'owner':'Minos','culture':'minoan'}
    tiles=[{'q':q,'r':r,'terrain':'plains','tier':'live'} for q in range(-4,8) for r in range(-4,6)]
    def api(route):
     path=route.request.url.split('/api/v1/')[-1]
     if path.endswith('/provinces'):data=[city]
     elif path.endswith('/map'):data=tiles
     elif path.endswith('/units'):data={'units':[{'id':'unit','type':'spearman','category':'land','status':'garrison','size':100,'settlement_id':'home-settlement','display_name':'1st Spearmen of Knossos','deployable':True}],'expedition_rules':{'min_ticks':4,'max_ticks':365,'default_ticks':10,'area_radius':5}}
     elif path.endswith('/cities'):data=[{'settlement_id':'home-settlement','name':'Knossos','own':True,'knowledge':'known','q':0,'r':0,'terrain':'plains'},{'settlement_id':'pylos','name':'Pylos','owner':'Nestor','knowledge':'known','q':4,'r':0,'terrain':'plains'}]
     elif '/march-preview?' in path:data={'available':True,'arrival_tick':4,'duration_ticks':4}
     elif '/notifications?' in path:data={'notifications':[{'id':'notice','kind':'UnitArrived','body':{'unit_type':'Spearman','q':4,'r':0},'created_at':'2026-10-22T09:00:00Z'}]}
     elif path.endswith('/roads'):data={'edges':[]}
     elif path.endswith('/road-sites'):data={'sites':[]}
     elif path.endswith('/provinces/home'):data={'settlement':{**city,'buildings':[],'build_queue':[],'resources':{},'labor':{}}}
     elif path.endswith('/placement-options'):data={'hexes':[],'buildings':[]}
     else:data=[]
     route.fulfill(content_type='application/json',body=json.dumps(data))
    page.route('**/api/v1/**',api)
    page.add_init_script("window.requestAnimationFrame=()=>0;window.setInterval=()=>0;Date.now=()=>Date.parse('2026-10-22T10:00:00Z');")
    page.goto(f'http://127.0.0.1:{server.server_port}/fixture')
    point=page.evaluate('''async args=>{
     await import('/static/js/megaron/main.js');
     const {State}=await import('/static/js/megaron/state.js');window.__state=State;
     const map=await import('/static/js/megaron/render/map.js');window.__map=map;
     const march=await import('/static/js/megaron/ui/marchctx.js');window.chooseMarchTarget=march.chooseMarchTarget;
     const dispatch=await import('/static/js/megaron/ui/dispatch_window.js');window.openDispatchWindow=dispatch.openDispatchWindow;
     document.getElementById('gt-wanax').textContent='Minos';State.WORLD_ID='fixture';State.MY_PLAYER_ID='minos';State.MY_SETTLEMENT_ID='home-settlement';State.provinceData=[args.city];State.tileData=args.tiles;State.unitsData=[];State.CURRENT_TICK=0;State.TICK_SECONDS=60;State.TICK_ANCHOR_MS=Date.now();
     map.initMap();await map.loadMap();State.camera.zoom=1;State.camera.x=150;State.camera.y=160;
     State.animFrame=63;for(let i=0;i<40;i++){State.dirty=true;map.render();}
     const c=document.getElementById('hex-canvas'),r=c.getBoundingClientRect(),p=map.hexPx(1,0);
     return {x:r.x+State.camera.x+p.x*map.SCALE,y:r.y+State.camera.y+p.y*map.SCALE};
    }''',{'city':city,'tiles':tiles})
    page.evaluate('document.fonts.ready')
    prefix=f'firefox-{width}-'+('baseline-' if baseline else '')
    def shot(name):
     page.wait_for_load_state('networkidle')
     # Visibility does not mean the drawer's 180ms CSS transition has ended.
     # Finish finite CSS animations at their real endpoint; rAF scenes are
     # already fixed at their initial frame by this explicit fixture.
     page.evaluate("document.getAnimations().filter(a=>Number.isFinite(a.effect.getComputedTiming().endTime)).forEach(a=>a.finish())")
     data=page.screenshot()
     digest=hashlib.sha256(Image.open(io.BytesIO(data)).tobytes()).hexdigest()
     proof.append({'width':width,'run':run,'scene':name,'rgba_sha256':digest})
     key=(width,name)
     if run==1:pixels[key]=digest
     else:
      assert pixels[key]==digest,(key,'non-deterministic pixels')
      (OUT/(prefix+name+'.png')).write_bytes(data)
    def check(selector):
     expect(page.locator(selector)).to_be_visible()
     if width==390 and not baseline:
      expect(page.locator(selector)).to_have_class(re.compile('mobile-fullscreen'))
      box=page.locator(selector).bounding_box();assert box=={'x':0,'y':0,'width':390,'height':844},box
      assert page.locator('.mobile-fullscreen').count()==1
      assert page.locator('#hex-canvas').evaluate('(e)=>e.inert')
    page.locator('#trig-city').tap();check('#drawer-city');expect(page.locator('#city-scene')).to_be_visible();expect(page.locator('#city-body')).to_contain_text('Population');shot('city')
    if width==390 and not baseline:
     page.locator('#city-body').evaluate('(e)=>e.scrollTop=e.scrollHeight')
     box=page.locator('#drawer-city .drawer-close').bounding_box();assert box['y']>=0 and box['y']+box['height']<=844,box
     page.locator('#city-body').evaluate('(e)=>e.scrollTop=0')
    page.locator('#drawer-city .drawer-close').tap();page.wait_for_timeout(250)
    page.touchscreen.tap(point['x'],point['y']);check('#inspect-panel');shot('hex')
    page.locator('#inspect-panel .inspect-close').tap()
    # Open through a real touch button, then choose a known city through the real select.
    page.locator('#trig-war').tap();expect(page.locator('#war-body')).to_contain_text('1st Spearmen')
    page.locator('#war-body button').filter(has_text=re.compile('^March')).first.tap();check('#march-ctx');expect(page.locator('#mctx-city')).to_be_enabled();page.select_option('#mctx-city','pylos');expect(page.locator('#mctx-name')).to_have_text('Pylos');expect(page.locator('#mctx-eta')).to_contain_text('Arrives in');shot('march')
    page.locator('#march-ctx .mctx-close').tap()
    page.locator('#gt-notif-btn').tap();check('#drawer-notif');shot('drawer')
    # Open actual notification from a touch target while the drawer is still open.
    page.locator('#notif-body [data-idx]').tap();check('#dispatch-window-overlay');shot('notice')
    if width==390 and not baseline:expect(page.locator('#drawer-notif')).not_to_have_class(re.compile(r'\bopen\b'))
    page.locator('#dispatch-window-overlay .drawer-close').tap()
    if width==390 and not baseline:
     assert not page.locator('#hex-canvas').evaluate('(e)=>e.inert')
     page.touchscreen.tap(point['x'],point['y']);check('#inspect-panel');page.locator('#inspect-panel .inspect-close').tap()
    page.locator('#gt-wanax').tap();check('#account-window-overlay');shot('account')
    page.locator('#account-window-overlay .drawer-close').tap()
    page.locator('.gt-search-btn').tap();check('#search-overlay');shot('search')
    page.locator('#search-overlay .si-esc').tap()
    if not baseline:
     assert not page.locator('#hex-canvas').evaluate('(e)=>e.inert')
     assert 'mobile-panel-blocked' not in page.locator('#map-root').get_attribute('class')
     page.evaluate('toggleSearch();toggleSearch()')
     assert not page.locator('#hex-canvas').evaluate('(e)=>e.inert')
    page.locator('#gt-codex-btn').tap();check('#codex-panel');expect(page.locator('#codex-search')).to_be_visible();shot('codex')
    page.locator('#codex-panel .drawer-close').tap()
    # Exercise the actual close entry points with an empty panel registry.
    if not baseline:
     page.evaluate("closeDrawer('notif');closeDrawer('war')")
     def unblocked():
      assert not page.locator('#hex-canvas').evaluate('(e)=>e.inert')
      assert 'mobile-panel-blocked' not in page.locator('#map-root').get_attribute('class')
      assert page.locator('.mobile-fullscreen').count()==0
     for close in ('toggle','escape','slash','backdrop','result'):
      page.locator('.gt-search-btn').tap();check('#search-overlay')
      if close=='toggle':page.evaluate('toggleSearch()')
      elif close=='escape':page.keyboard.press('Escape')
      elif close=='slash':
       page.locator('#search-input').evaluate('(e)=>e.blur()');page.keyboard.press('/')
      elif close=='backdrop':page.locator('#search-overlay').dispatch_event('click')
      else:page.locator('#search-results .sr-item').first.tap()
      expect(page.locator('#search-overlay')).not_to_be_visible();unblocked()
     for close in ('escape','outside'):
      page.evaluate("chooseMarchTarget('unit')");check('#march-ctx')
      if close=='escape':page.keyboard.press('Escape')
      else:page.locator('body').dispatch_event('mousedown')
      unblocked()
     for toggle in ("toggleDrawer('city')",'toggleCodex()','toggleAccountWindow()'):
      page.evaluate(toggle);page.evaluate(toggle);unblocked()
     for opener,selector in [('#gt-wanax','#account-window-overlay'),('#gt-codex-btn','#codex-panel'),('#trig-city','#drawer-city')]:
      page.locator(opener).tap();check(selector);page.keyboard.press('Escape');unblocked()
     # Fullscreen has no backdrop space; dispatch a real DOM backdrop click.
     page.locator('#gt-wanax').tap();page.locator('#account-window-overlay').dispatch_event('click');unblocked()
     page.evaluate("openDispatchWindow({kind:'UnitArrived',body:{q:4,r:0},created_at:'2026-10-22T09:00:00Z'})")
     page.keyboard.press('Escape');unblocked()
     page.evaluate("openDispatchWindow({kind:'UnitArrived',body:{q:4,r:0},created_at:'2026-10-22T09:00:00Z'})")
     page.locator('#dispatch-window-overlay').dispatch_event('click');unblocked()
    assert not errors,errors
    proof.append({'width':width,'run':run,'touch':True,'page_errors':errors})
    page.close()
  browser.close()
 print(json.dumps(proof,indent=2))
finally:server.shutdown();server.server_close()
