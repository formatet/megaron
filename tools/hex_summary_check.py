"""Render the real founding-site hex panel with an explicit forecast fixture.
Run: python3 tools/hex_summary_check.py [output-directory]
"""
import functools,http.server,json,re,sys,threading
from pathlib import Path
from urllib.parse import urlparse
from playwright.sync_api import sync_playwright,expect
ROOT=Path(__file__).resolve().parents[1];OUT=Path(sys.argv[1] if len(sys.argv)>1 else '/tmp/megaron-hex-summary');OUT.mkdir(parents=True,exist_ok=True)
class Handler(http.server.SimpleHTTPRequestHandler):
 def log_message(self,*_):pass
server=http.server.ThreadingHTTPServer(('127.0.0.1',0),functools.partial(Handler,directory=str(ROOT/'web')));threading.Thread(target=server.serve_forever,daemon=True).start()
url=f'http://127.0.0.1:{server.server_port}'
html=re.sub(r'<script\b[^>]*>.*?</script>','',(ROOT/'web/static/map.html').read_text(),flags=re.S)
forecast={'grain':{'base_per_tick':1.5,'fish_per_tick':4,'consumption_per_tick':5,'food_net_per_tick':0.5,'seed':1200},'goods':{'fish':4,'incense':1,'oil':5,'stone':4,'timber':1,'wine':5},'harbour_note':'Coastal: this hex touches water, so a city here can build a harbour and ships.','unknown_hexes':9,'catchment':[{'known':True,'terrain':'plains'}]+[{'known':False} for _ in range(9)]+[{'known':True,'terrain':'coastal_sea'} for _ in range(9)]}
try:
 with sync_playwright() as p:
  browser=p.chromium.launch();page=browser.new_page(viewport={'width':1280,'height':720},device_scale_factor=1)
  page.on('pageerror',lambda e:print('PAGE ERROR',str(e)))
  page.route('**/fixture',lambda r:r.fulfill(content_type='text/html',body=html))
  def api(r):
   assert r.request.method=='GET'
   path=urlparse(r.request.url).path
   data=forecast if '/colonize-preview' in path else ([{'q':0,'r':0,'terrain':'plains'}] if path.endswith('/map') else {'units':[]} if path.endswith('/units') else [])
   r.fulfill(content_type='application/json',body=json.dumps(data))
  page.route('**/api/**',api);page.goto(url+'/fixture',wait_until='domcontentloaded')
  page.evaluate('''async()=>{
   localStorage.setItem('poleia_token','hex-fixture');
   window.S=(await import('/static/js/megaron/state.js')).State;
   S.WORLD_ID='fixture';S.tileData=[{q:0,r:0,terrain:'plains'}];S.provinceData=[];S.unitsData=[];S.foreignUnitData=[];S.ruralData=[];
   S.founderPhase={active:true,population:1000,grain:{amount:1200}};S.camera={x:0,y:0,zoom:1};
   const menu=await import('/static/js/megaron/ui/marchctx.js');window.renderColonizePreviewHTML=menu.renderColonizePreviewHTML;window.MusicPlayer={update(){}};
   const map=await import('/static/js/megaron/render/map.js');map.initMap();await map.loadMap();
   const canvas=document.getElementById('hex-canvas');S.camera={x:canvas.clientWidth/2,y:canvas.clientHeight/2,zoom:1};
  }''')
  box=page.locator('#hex-canvas').bounding_box();page.mouse.click(box['x']+box['width']/2,box['y']+box['height']/2)
  expect(page.locator('#ip-found-summary')).to_contain_text('Feeds itself:')
  expect(page.locator('#ip-found-summary')).to_contain_text('10 of 19')
  details=page.locator('#ip-body-extra details');assert details.get_attribute('open') is None
  expect(page.locator('#ip-found-preview')).not_to_be_visible()
  page.locator('#inspect-panel').screenshot(path=str(OUT/'closed.png'))
  details.locator('summary').click();expect(page.locator('#ip-found-preview')).to_be_visible()
  expect(page.locator('#ip-found-preview')).to_contain_text('Catchment forecast')
  page.locator('#inspect-panel').screenshot(path=str(OUT/'details.png'))
  print('Closed summary and expanded original forecast verified; read-only API fixtures.')
  browser.close()
finally:server.shutdown()
