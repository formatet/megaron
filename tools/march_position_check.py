"""Measure real march controls during quantity changes; explicit API fixtures.
Run: python3 tools/march_position_check.py [output-directory]
"""
import functools,http.server,json,re,sys,threading
from pathlib import Path
from urllib.parse import urlparse
from playwright.sync_api import sync_playwright,expect
ROOT=Path(__file__).resolve().parents[1]
OUT=Path(sys.argv[1] if len(sys.argv)>1 else '/tmp/megaron-march-position');OUT.mkdir(parents=True,exist_ok=True)
class Handler(http.server.SimpleHTTPRequestHandler):
 def log_message(self,*_):pass
server=http.server.ThreadingHTTPServer(('127.0.0.1',0),functools.partial(Handler,directory=str(ROOT/'web')))
threading.Thread(target=server.serve_forever,daemon=True).start()
url=f'http://127.0.0.1:{server.server_port}'
html=re.sub(r'<script\b[^>]*>.*?</script>','',(ROOT/'web/static/map.html').read_text(),flags=re.S)
units=[dict(id=str(i),type='spearman',display_name=f'{i+1}st Spearmen of Knossos',category='land',status='garrison',deployable=True,settlement_id='city') for i in range(3)]
try:
 with sync_playwright() as p:
  browser=p.chromium.launch();page=browser.new_page(viewport={'width':1280,'height':720},device_scale_factor=1)
  page.route('**/fixture',lambda r:r.fulfill(content_type='text/html',body=html))
  def api(r):
   path=urlparse(r.request.url).path
   assert r.request.method=='GET','This proof must never send an order'
   if path.endswith('/units'):data={'units':units,'expedition_rules':{'min_ticks':4,'max_ticks':365,'default_ticks':10,'area_radius':5}}
   elif path.endswith('/march-preview'):data={'available':True,'arrival_tick':109,'duration_ticks':9,'arrives_at_utc':'2099-01-01T00:00:00Z'}
   else:data=[]
   r.fulfill(content_type='application/json',body=json.dumps(data))
  page.route('**/api/**',api)
  page.goto(url+'/fixture',wait_until='domcontentloaded')
  page.evaluate('''async()=>{
   localStorage.setItem('poleia_token','position-fixture');
   window.S=(await import('/static/js/megaron/state.js')).State;
   S.WORLD_ID='fixture';S.CURRENT_TICK=100;S.TICK_SECONDS=60;
   S.provinceData=[{id:'city',settlement_id:'city',name:'Knossos',q:0,r:0}];
   window.march=await import('/static/js/megaron/ui/marchctx.js');
   await march.openMarchCtx({q:3,r:2,known:true,isSea:false,isSettlement:true,name:'Knossos'},1100,650);
  }''')
  expect(page.locator('#mctx-eta')).to_contain_text('3st Spearmen')
  def geometry():return page.locator('#march-ctx').evaluate('(e)=>({top:e.getBoundingClientRect().top,left:e.getBoundingClientRect().left,inputTop:document.getElementById("mg-0").getBoundingClientRect().top,bottom:e.getBoundingClientRect().bottom})')
  measurements=[geometry()];page.locator('#march-ctx').screenshot(path=str(OUT/'three.png'))
  page.locator('#mg-0').fill('0');expect(page.locator('#mctx-eta')).not_to_be_visible();measurements.append(geometry())
  for n in [1,3,0,2]:
   page.locator('#mg-0').fill(str(n))
   if n:expect(page.locator('#mctx-eta')).to_contain_text('Arrives in')
   else:expect(page.locator('#mctx-eta')).not_to_be_visible()
   measurements.append(geometry())
  page.locator('#march-ctx').screenshot(path=str(OUT/'two.png'))
  (OUT/'geometry.json').write_text(json.dumps(measurements,indent=2)+'\n');print(json.dumps(measurements))
  assert all(abs(m['top']-measurements[0]['top'])<0.5 and abs(m['left']-measurements[0]['left'])<0.5 and abs(m['inputTop']-measurements[0]['inputTop'])<0.5 for m in measurements),'Quantity controls moved'
  assert all(m['bottom']<=712.5 for m in measurements),'Menu extends below viewport'
  browser.close()
finally:server.shutdown()
