#!/usr/bin/env python3
"""A4 BILD: the REAL Diplomacy drawer (diplomacy.js + diplomacy_runners.js + megaron.css)
in Firefox over a stubbed /runners answer with human names. The fixture is the server's
JSON shape (docs/reviews/runners-flik/README.md); nothing but the HTTP answer is faked.

    python3 tools/runners_flik_shot.py [outdir]
"""
import functools, http.server, json, re, sys, threading
from datetime import datetime, timedelta
from pathlib import Path
from playwright.sync_api import sync_playwright

ROOT = Path(__file__).resolve().parents[1]
OUT = Path(sys.argv[1]) if len(sys.argv) > 1 else ROOT / 'docs/reviews/runners-flik'
OUT.mkdir(parents=True, exist_ok=True)


class Handler(http.server.SimpleHTTPRequestHandler):
    def log_message(self, *a): pass


server = http.server.ThreadingHTTPServer(('127.0.0.1', 0), functools.partial(Handler, directory=str(ROOT / 'web')))
threading.Thread(target=server.serve_forever, daemon=True).start()
base = f'http://127.0.0.1:{server.server_port}'
html = re.sub(r'<script\b[^>]*>.*?</script>', '', (ROOT / 'web/static/map.html').read_text(), flags=re.S)

now = datetime.now().astimezone()
iso = lambda h: (now + timedelta(hours=h)).isoformat()
SORRY = 'Your runner is waiting in Phaistos: no ship has sailed from there toward Zakros. Any ship that does — yours or another Wanax’s — takes it aboard by itself. You can also send one of your own ships now.'
OWNPORT = 'Your runner is waiting in Zakros: no ship has sailed from there toward Amnissos. Any ship that does — yours or another Wanax’s — takes it aboard by itself. None of your ships is free in this port; build or send one here to carry it sooner. Or call it back home, undelivered.'
DATA = {
    'own': [
        {'id': '11111111-0000-0000-0000-000000000001', 'group': 'waiting', 'kind': 'letter', 'returning': True, 'to_city': 'Zakros', 'to_wanax': None, 'port_name': 'Phaistos',
         'waiting_since': iso(-4.2), 'explanation': SORRY, 'can_call_back': False, 'can_arrange_passage': True,
         'eligible_ships': [{'id': 's1', 'name': 'Dawn Gull', 'type': 'merchantman', 'settlement_id': 'x', 'settlement_name': 'Zakros'},
                            {'id': 's2', 'name': 'Salt Wind', 'type': 'galley', 'settlement_id': 'y', 'settlement_name': 'Kommos'}]},
        {'id': '11111111-0000-0000-0000-000000000002', 'group': 'waiting', 'kind': 'offer', 'to_city': 'Amnissos', 'to_wanax': 'Kassandra', 'port_name': 'Zakros',
         'waiting_since': iso(-1.5), 'explanation': OWNPORT, 'can_call_back': True, 'can_arrange_passage': True, 'eligible_ships': []},
        {'id': '11111111-0000-0000-0000-000000000003', 'group': 'on_the_way', 'kind': 'order', 'order_summary': 'Third Spears: March to (14,-3); mission: scout', 'arrives_at': iso(1.4), 'can_call_back': False, 'can_arrange_passage': False},
        {'id': '11111111-0000-0000-0000-000000000004', 'group': 'on_the_way', 'kind': 'letter', 'to_city': 'Knossos', 'to_wanax': 'Dunios', 'arrives_at': iso(6), 'can_call_back': False, 'can_arrange_passage': False},
        {'id': '11111111-0000-0000-0000-000000000005', 'group': 'aboard', 'kind': 'letter', 'to_city': 'Amnissos', 'to_wanax': 'Kassandra', 'ship_name': 'Sea Swallow', 'ship_wanax': 'Dunios',
         'boarded_at': iso(-2), 'lands_at': iso(9), 'arrives_at': iso(11), 'can_call_back': False, 'can_arrange_passage': False},
        {'id': '11111111-0000-0000-0000-000000000007', 'group': 'awaiting_answer', 'kind': 'offer', 'to_city': 'Knossos', 'to_wanax': 'Dunios',
         'waiting_since': iso(-7), 'leaves_at': iso(161), 'can_call_back': False, 'can_arrange_passage': False},
        {'id': '11111111-0000-0000-0000-000000000008', 'group': 'awaiting_answer', 'kind': 'letter', 'to_city': 'Amnissos', 'to_wanax': 'Kassandra',
         'waiting_since': iso(-30), 'leaves_at': iso(18), 'can_call_back': False, 'can_arrange_passage': False},
        {'id': '11111111-0000-0000-0000-000000000006', 'group': 'homebound', 'kind': 'letter', 'to_city': 'Zakros', 'arrives_at': iso(3), 'can_call_back': False, 'can_arrange_passage': False},
    ],
    'foreign': [
        {'group': 'in_your_city', 'sender_wanax': 'Dunios', 'where': 'Zakros', 'waiting_for': 'a_ship', 'since': iso(-5)},
        {'group': 'in_your_city', 'sender_wanax': 'Kassandra', 'where': 'Phaistos', 'waiting_for': 'your_answer'},
        {'group': 'aboard_your_ship', 'sender_wanax': 'Kassandra', 'ship_name': 'Dawn Gull', 'heading_to': 'Amnissos', 'lands_at': iso(8)},
    ],
}

with sync_playwright() as pw:
    browser = pw.firefox.launch()
    for code, width, height, skip in [('A1', 800, 1200, 0), ('A2', 390, 1260, 0)]:
        ctx = browser.new_context(viewport={'width': width, 'height': height}, timezone_id='Europe/Stockholm', locale='en-GB')
        page = ctx.new_page(); errors = []
        page.on('pageerror', lambda e: errors.append(str(e)))
        page.route('**/fixture', lambda r: r.fulfill(content_type='text/html', body=html))
        def api(route):
            url = route.request.url
            body = DATA if url.endswith('/runners') else []
            route.fulfill(content_type='application/json', body=json.dumps(body))
        page.route('**/api/**', api)
        page.goto(base + '/fixture', wait_until='domcontentloaded')
        page.evaluate('''async()=>{
          localStorage.setItem('poleia_token','shot');
          const {State}=await import('/static/js/megaron/state.js');
          State.WORLD_ID='w';State.MY_SETTLEMENT_ID='zakros';State.provinceData=[];
          const d=await import('/static/js/megaron/ui/drawers/diplomacy.js');
          document.getElementById('drawer-diplomacy').classList.add('open');
          await d.loadDiplomacyDrawer();
          document.querySelector('.dtab[data-tab="runners"]').click();
          await new Promise(r=>setTimeout(r,400));
        }''')
        page.wait_for_selector('#dtab-runners .runner-row')
        drawer = page.locator('#drawer-diplomacy')
        drawer.screenshot(path=str(OUT / f'{code}.png'))
        assert not errors, errors
        print(code, width, drawer.bounding_box())
        ctx.close()
    browser.close()
server.shutdown()
