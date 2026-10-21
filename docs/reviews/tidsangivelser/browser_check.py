#!/usr/bin/env python3
"""Local Firefox integration proof; imports the branch's actual web modules."""
from pathlib import Path
from functools import partial
from http.server import ThreadingHTTPServer, SimpleHTTPRequestHandler
from threading import Thread
import json
from playwright.sync_api import sync_playwright

ROOT = Path(__file__).resolve().parents[3]
class QuietHandler(SimpleHTTPRequestHandler):
    def log_message(self, *_):
        pass
server = ThreadingHTTPServer(('127.0.0.1', 0), partial(QuietHandler, directory=str(ROOT)))
Thread(target=server.serve_forever, daemon=True).start()
try:
    with sync_playwright() as pw:
        browser = pw.firefox.launch(headless=True)
        context = browser.new_context(locale='en-GB', timezone_id='Europe/Stockholm')
        page = context.new_page()
        errors = []
        page.on('pageerror', lambda error: errors.append(str(error)))
        page.goto(f'http://127.0.0.1:{server.server_port}/')
        result = page.evaluate('''async () => {
          const NativeDate=Date, now=NativeDate.parse('2026-10-21T21:50:00Z');
          globalThis.Date=class extends NativeDate {
            constructor(...args){super(...(args.length?args:[now]));}
            static now(){return now;}
          };
          const {State}=await import('/web/static/js/megaron/state.js');
          const {expeditionMissionText}=await import('/web/static/js/megaron/ui/expedition.js');
          const {notificationDay}=await import('/web/static/js/megaron/ui/notification_state.js');
          const {notifText}=await import('/web/static/js/megaron/ui/format.js');
          Object.assign(State,{CURRENT_TICK:100,TICK_SECONDS:60,TICK_ANCHOR_MS:now});
          const expedition=expeditionMissionText({area_q:16,area_r:29,length_ticks:30,turn_tick:120,home_by_tick:135});
          const recorded=notificationDay({world_tick:0,created_at:'2020-01-01T00:00:00Z'});
          const historical=notifText('MessengerRescuedAtSea',{home_tick:0,journey:[{ship:'Dolphin'},{port:'Tiryns'}]});
          document.body.replaceChildren();
          for(const text of [expedition,recorded,historical]) {const p=document.createElement('p');p.textContent=text;document.body.append(p);}
          return {expedition,recorded,historical,text:document.body.innerText};
        }''')
        assert result['expedition'] == 'Expedition around (16,29), 30 days; turns home by tomorrow 00:10; home by tomorrow 00:25.', result
        assert result['recorded'] == 'today 22:10', result
        assert result['historical'] == 'Home today 22:10. Your runner was rescued at sea by the Dolphin and put ashore at Tiryns.', result
        assert not errors, errors
        result['browser'] = 'Firefox'
        result['page_errors'] = errors
        print(json.dumps(result, indent=2))
        browser.close()
finally:
    server.shutdown()
    server.server_close()
