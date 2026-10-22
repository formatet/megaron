#!/usr/bin/env python3
"""Render production presentation modules in Firefox; not a player comprehension verdict."""
import functools, http.server, json, threading
from pathlib import Path
from playwright.sync_api import sync_playwright
root=Path(__file__).resolve().parents[3]
class Quiet(http.server.SimpleHTTPRequestHandler):
    def log_message(self,*args): pass
server=http.server.ThreadingHTTPServer(('127.0.0.1',0),functools.partial(Quiet,directory=str(root/'web/static')))
threading.Thread(target=server.serve_forever,daemon=True).start()
try:
    with sync_playwright() as p:
        browser=p.firefox.launch(headless=True)
        page=browser.new_page()
        errors=[]
        page.on('pageerror',lambda e:errors.append(str(e)))
        page.goto(f'http://127.0.0.1:{server.server_port}/')
        result=page.evaluate('''async()=>{
          const {State}=await import('/js/megaron/state.js');
          State.CURRENT_TICK=0;State.TICK_SECONDS=60;State.TICK_ANCHOR_MS=Date.now();
          const {expeditionOrderText,expeditionMissionText}=await import('/js/megaron/ui/expedition.js');
          const {marchPreviewHTML}=await import('/js/megaron/ui/march_preview.js');
          const mission={area_q:24,area_r:20,length_ticks:100,turn_tick:50,home_by_tick:100};
          const result={order:expeditionOrderText(24,20,100,{area_radius:5}),receipt:expeditionMissionText({...mission,first_to_area:true}),legacy:expeditionMissionText(mission),preview:marchPreviewHTML({available:false,reason:'unknown_terrain',planned_expedition_ticks:100,planned_first_target_q:24,planned_first_target_r:20})};
          document.body.replaceChildren();
          for(const [name,text]of Object.entries(result)){const row=document.createElement('div');row.dataset.name=name;row.innerHTML=text;document.body.append(row);}
          return Object.fromEntries([...document.querySelectorAll('[data-name]')].map(e=>[e.dataset.name,e.innerText]));
        }''')
        assert 'first to (24,20), then explores around it' in result['receipt']
        assert 'first to (24,20), then explores around it' in result['preview']
        assert 'First-leg arrival remains unknown' in result['preview']
        assert 'first to' not in result['legacy']
        assert not errors,errors
        print(json.dumps({'browser':'Firefox','scope':'production text modules only; not the whole order UI or live API','text':result,'pageerrors':errors},indent=2))
        browser.close()
finally:
    server.shutdown();server.server_close()
