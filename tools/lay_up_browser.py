#!/usr/bin/env python3
"""Firefox BILD: actual War card renderer and CSS, explicit local fixture."""
from pathlib import Path
import functools
import http.server
import json
import threading
from playwright.sync_api import sync_playwright

ROOT = Path(__file__).resolve().parents[1]
OUT = ROOT / 'docs/reviews/lay-up/browser'
OUT.mkdir(parents=True, exist_ok=True)
HTML = '''<!doctype html><html><head><meta charset="utf-8"><link rel="stylesheet" href="/static/megaron.css"></head>
<body style="background:#ded8ca;padding:16px"><div id="map-root" style="display:none"><canvas id="hex-canvas"></canvas></div><div id="march-ctx" style="display:none"></div><main style="width:360px;max-width:100%;background:#f5f0e6;padding:12px;border:1px solid #aaa">
<h2 style="font-size:1rem">Army · harbour ships</h2><div id="cards"></div></main></body></html>'''

class Handler(http.server.SimpleHTTPRequestHandler):
    def log_message(self, *_): pass
    def do_GET(self):
        if self.path == '/fixture':
            self.send_response(200)
            self.send_header('Content-Type', 'text/html; charset=utf-8')
            self.end_headers()
            self.wfile.write(HTML.encode())
            return
        super().do_GET()

server = http.server.ThreadingHTTPServer(('127.0.0.1', 0), functools.partial(Handler, directory=str(ROOT / 'web')))
threading.Thread(target=server.serve_forever, daemon=True).start()
try:
    with sync_playwright() as pw:
        browser = pw.firefox.launch()
        for width in (800, 390):
            page = browser.new_page(viewport={'width': width, 'height': 540}, device_scale_factor=1)
            errors = []
            page.on('pageerror', lambda error: errors.append(str(error)))
            page.goto(f'http://127.0.0.1:{server.server_port}/fixture')
            page.evaluate('''async () => {
              const {State} = await import('/static/js/megaron/state.js');
              const {renderUnitCard} = await import('/static/js/megaron/ui/drawers/war.js');
              State.provinceData = [{id:'port',settlement_id:'port',name:'Pylos',own:true,is_capital:true}];
              const common = {type:'galley',category:'naval',size:1,crew:20,hull:5,settlement_id:'port',
                upkeep_food_per_tick:.04,upkeep_silver_per_tick:1.5,upkeep_context:'active'};
              const active={...common,id:'active',display_name:'White Dolphin, Galley of Pylos',status:'garrison',deployable:true,can_lay_up:true};
              const stored={...common,id:'stored',display_name:'Blue Heron, Galley of Pylos',status:'laid_up',deployable:false,
                upkeep_food_per_tick:0,upkeep_silver_per_tick:0,upkeep_context:'inactive',can_fit_out:true};
              const preparing={...common,id:'preparing',display_name:'Asterion, Galley of Pylos',status:'fitting_out',deployable:false,
                build_complete_at:'2026-10-24T12:00:00Z'};
              document.querySelector('#cards').innerHTML=[active,stored,preparing].map(renderUnitCard).join('');
            }''')
            text = page.locator('#cards').inner_text()
            assert 'Lay up' in text and 'Fit out' in text and 'Laid up' in text and 'fitting out' in text, text
            assert not errors, errors
            page.screenshot(path=str(OUT / f'firefox-{width}.png'))
            page.close()
        browser.close()
finally:
    server.shutdown()
print(json.dumps({'browser':'Firefox','widths':[800,390],'cards':['active','laid_up','fitting_out'],'result':'PASS'}))
