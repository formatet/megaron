#!/usr/bin/env python3
"""Actual map renderer, Host marker/panel/hover, real styles, isolated API fixture."""
import functools,http.server,json,re,threading,hashlib
from pathlib import Path
from playwright.sync_api import sync_playwright,expect
ROOT=Path(__file__).resolve().parents[3];OUT=Path(__file__).resolve().parent
class Handler(http.server.SimpleHTTPRequestHandler):
 def log_message(self,*args):pass
server=http.server.ThreadingHTTPServer(('127.0.0.1',0),functools.partial(Handler,directory=str(ROOT/'web')))
threading.Thread(target=server.serve_forever,daemon=True).start()
# Keep actual map DOM/styles; bootstrap is replaced by explicit deterministic fixture setup.
html=re.sub(r'<script\b[^>]*>.*?</script>','',(ROOT/'web/static/map.html').read_text(),flags=re.S)
proof=[]
try:
 with sync_playwright() as pw:
  browser=pw.firefox.launch()
  for width in (1000,390):
   for status,can,terrain in [('marching',True,'plains'),('waiting',False,'semi_desert'),('can_found_here',True,'plains')]:
    for run in (1,2):
     page=browser.new_page(viewport={'width':width,'height':900},device_scale_factor=1);errors=[];posts=[]
     page.on('pageerror',lambda e:errors.append(str(e)))
     state={'q':2,'r':0,'at_milli':3000,'status':status,'can_found_here':can}
     if not can:state['reason']='This ground cannot support a metropolis.'
     phase={'active':True,'host_unit_id':'host','q':2,'r':0,'host_state':state,'population':1000,'spearmen_in_field':2,'grain':{'amount':1200,'ticks_left':None},'silver':{'amount':60,'ticks_left':None}}
     host={'id':'host','type':'nomadic_host','category':'land','display_name':'Nomadic Host of Minos','status':'marching' if status=='marching' else 'positioned','q':0 if status=='marching' else 2,'r':0,'target_q':3,'target_r':0,'current_q':2,'current_r':0,'host_state':state,'departure_tick':0,'arrival_tick':6,'departs_at':'2026-10-22T07:00:00Z','arrives_at':'2026-10-22T13:00:00Z','path':[[0,0],[1,0],[2,0],[3,0]],'step_costs':[1000,3000,2000],'deployable':True}
     tiles=[{'q':q,'r':r,'terrain':terrain if(q,r)==(2,0)else'plains','tier':'live'} for q in range(-3,8) for r in range(-4,5)]
     page.route('**/fixture',lambda route:route.fulfill(content_type='text/html',body=html))
     def api(route):
      path=route.request.url.split('/api/v1/')[-1]
      if route.request.method=='POST':posts.append(route.request.post_data_json);data={}
      elif path.endswith('/founding/status'):data=phase
      elif path.endswith('/units'):data={'units':[host]}
      elif path.endswith('/map'):data=tiles
      elif '/colonize-preview?' in path:data={'grain':{'food_net_per_tick':1},'goods':{'timber':2,'stone':1},'catchment':[],'harbour_note':'Inland: no harbour'}
      elif path.endswith('/roads'):data={'edges':[]}
      elif path.endswith('/road-sites'):data={'sites':[]}
      elif path.endswith('/settlements/placement-roster'):data=[]
      else:data=[]
      route.fulfill(status=200,content_type='application/json',body=json.dumps(data))
     page.route('**/api/v1/**',api)
     page.add_init_script("window.requestAnimationFrame=()=>0;window.setInterval=()=>0;Date.now=()=>Date.parse('2026-10-22T10:00:00Z');")
     page.goto(f'http://127.0.0.1:{server.server_port}/fixture')
     point=page.evaluate('''async args=>{
      const {State}=await import('/static/js/megaron/state.js');
      const map=await import('/static/js/megaron/render/map.js');window.__map=map;window.__state=State;
      const march=await import('/static/js/megaron/ui/marchctx.js');Object.assign(window,march);
      const misc=await import('/static/js/megaron/ui/misc.js');window.MusicPlayer=misc.MusicPlayer;
      State.WORLD_ID='fixture';State.MY_SETTLEMENT_ID=null;State.founderPhase=args.phase;State.unitsData=[args.host];State.CURRENT_TICK=3;State.TICK_SECONDS=3600;State.TICK_ANCHOR_MS=Date.now();
      map.initMap();await map.loadMap();
      State.camera.zoom=1;const canvas=document.getElementById('hex-canvas');const p=map.hexPx(2,0);
      State.camera.x=canvas.width*.5-p.x*map.SCALE;State.camera.y=canvas.height*.4-p.y*map.SCALE;
      State.animFrame=63;State.dirty=true;map.render();
      const rect=canvas.getBoundingClientRect();return {x:rect.x+State.camera.x+p.x*map.SCALE,y:rect.y+State.camera.y+p.y*map.SCALE};
     }''',{'phase':phase,'host':host})
     # Real pointer hover and click at the server-owned physical hex, not departure.
     page.mouse.move(point['x'],point['y'])
     expect(page.locator('#tile-tooltip')).to_contain_text({'marching':'Marching','waiting':'Waiting','can_found_here':'Can found here'}[status])
     hover=page.locator('#tile-tooltip').inner_text()
     page.mouse.click(point['x'],point['y'])
     expect(page.locator('#ip-name')).to_have_text('Nomadic Host')
     expect(page.locator('#ip-host-state')).to_contain_text('(2,0)')
     expect(page.locator('#ip-host-state')).to_contain_text({'marching':'Marching','waiting':'Waiting','can_found_here':'Can found here'}[status])
     if can:expect(page.locator('#ip-settle-btn')).to_be_enabled()
     else:expect(page.locator('#ip-settle-btn')).to_be_disabled()
     expect(page.locator('#ip-found-summary')).not_to_contain_text('Fetching')
     point=page.evaluate('''async()=>{
      const s=window.__state,m=window.__map,canvas=document.getElementById('hex-canvas'),p=m.hexPx(2,0);
      const panel=document.getElementById('inspect-panel').getBoundingClientRect();
      s.camera.x=Math.max(40,panel.left*.5)-p.x*m.SCALE;
      for(let i=0;i<40;i++){s.animFrame=63;s.dirty=true;m.render();await new Promise(resolve=>setTimeout(resolve,1));}
      const rect=canvas.getBoundingClientRect();return {x:rect.x+s.camera.x+p.x*m.SCALE,y:rect.y+s.camera.y+p.y*m.SCALE};
     }''')
     page.mouse.move(point['x'],point['y'])
     expect(page.locator('#tile-tooltip')).to_contain_text({'marching':'Marching','waiting':'Waiting','can_found_here':'Can found here'}[status])
     assert not posts,posts
     if run==2:page.screenshot(path=str(OUT/f'firefox-{width}-{status}.png'))
     assert not errors,errors
     proof.append({'width':width,'status':status,'run':run,'physical_hex':[2,0],'stored_departure':[host['q'],0],'hover':hover,'panel_status':page.locator('#ip-host-state').inner_text(),'can_found_here':can,'page_errors':errors})
     page.close()
  browser.close()
 print(json.dumps(proof,indent=2))
finally:server.shutdown();server.server_close()
