#!/usr/bin/env python3
"""Render the merged notification list in real Chromium with stubbed HTTP.
Usage: en_lista_shot.py OUT_DIR. Isolated fixture, not a real-game proof."""
import functools,http.server,pathlib,sys,threading
from playwright.sync_api import sync_playwright
ROOT=pathlib.Path(__file__).resolve().parents[1]
class Q(http.server.SimpleHTTPRequestHandler):
 def log_message(self,*a):pass
srv=http.server.ThreadingHTTPServer(('127.0.0.1',0),functools.partial(Q,directory=str(ROOT)))
threading.Thread(target=srv.serve_forever,daemon=True).start()
out=pathlib.Path(sys.argv[1]);out.mkdir(parents=True,exist_ok=True)
PAGE='''<!doctype html><link rel="stylesheet" href="/web/static/megaron.css"><body><span id="gt-notif-badge" style="display:none"></span><div class="drawer open" id="drawer-notif" style="position:static;width:420px"><div class="drawer-header"><span class="drawer-title">Notifications</span></div><div class="drawer-body" id="notif-body"></div></div>'''
with sync_playwright() as p:
 b=p.chromium.launch(headless=True,ignore_default_args=['--disable-dev-shm-usage'])
 for name,vp in (('desktop',{'width':460,'height':700}),('mobile',{'width':390,'height':700})):
  pg=b.new_page(viewport=vp);pg.on('console',lambda m:print('console:',m.text) if m.type=='error' else None);pg.on('pageerror',lambda e:print('pageerror:',e));pg.route('**/shot.html',lambda r:r.fulfill(body=PAGE,content_type='text/html'));pg.goto(f'http://127.0.0.1:{srv.server_port}/shot.html')
  pg.evaluate('''async()=>{
   window.addEventListener=window.addEventListener;
   const {State}=await import('/web/static/js/megaron/state.js');State.WORLD_ID='W';State.TICK_ANCHOR_MS=Date.now();State.TICK_SECONDS=3600;
   const now=Date.now(),iso=m=>new Date(now-m*60000).toISOString();
   window.fetch=async(url)=>{const j=d=>new Response(JSON.stringify(d));
    if(url.includes('/gossip'))return j([{source_region:'Southern coast',category:'war',text:'Ships were seen burning off the headland.',hops:2,importance:'major',generated_at:iso(20)},{source_region:'The delta',category:'trade',text:'A new city is said to stand east of the river mouth.',hops:1,generated_at:iso(200)}]);
    if(url.includes('/notifications'))return j({notifications:[{kind:'ColonyFounded',level:3,created_at:iso(5),body:{name:'Nostos'}},{kind:'MessengerArrived',level:3,created_at:iso(90),body:{}}]});
    return j([]);};
   const m=await import('/web/static/js/megaron/ui/drawers/notif.js');await m.loadNotifDrawer();}''')
  pg.wait_for_timeout(300);pg.screenshot(path=str(out/f'list-{name}.png'));pg.close()
 b.close()
srv.shutdown()
