"""Browser regression with explicit API fixtures; no game-world mutations.
Runs actual map DOM, search module, fetchAuth and destination drawer renderers.
Usage: python3 tools/search_messages_acceptance.py [output-dir]
"""
import functools
import http.server
import json
from pathlib import Path
import re
import sys
import threading
from playwright.sync_api import sync_playwright, expect

ROOT = Path(__file__).resolve().parents[1]
OUT = Path(sys.argv[1] if len(sys.argv) > 1 else '/tmp/megaron-search-browser')
OUT.mkdir(parents=True, exist_ok=True)

class Handler(http.server.SimpleHTTPRequestHandler):
    def log_message(self, *_):
        pass

server = http.server.ThreadingHTTPServer(('127.0.0.1', 0), functools.partial(Handler, directory=str(ROOT / 'web')))
threading.Thread(target=server.serve_forever, daemon=True).start()
url = f'http://127.0.0.1:{server.server_port}'
html = re.sub(r'<script\b[^>]*>.*?</script>', '', (ROOT / 'web/static/map.html').read_text(), flags=re.S)
letters = [{'from_name': 'Knossos', 'to_name': 'Petras', 'message': 'We need copper <script>window.attacked=true</script>', 'status': 'delivered'}]
rumours = [{'source_region': 'Northern coast', 'category': 'trade', 'text': 'Tin is scarce', 'hops': 1}]
requests = []
errors = []
try:
    with sync_playwright() as p:
        browser = p.chromium.launch()
        page = browser.new_page(viewport={'width': 1280, 'height': 900})
        page.on('pageerror', lambda e: errors.append(str(e)))
        page.route('**/fixture', lambda route: route.fulfill(content_type='text/html', body=html))
        def api(route):
            req = route.request
            assert req.headers.get('authorization') == 'Bearer search-fixture'
            requests.append(req.url)
            if req.url.endswith('/inbox'):
                data = letters
            elif req.url.endswith('/gossip'):
                data = rumours
            else:
                data = []
            route.fulfill(content_type='application/json', body=json.dumps(data))
        page.route('**/api/**', api)
        page.goto(url + '/fixture')
        page.evaluate('''async () => {
          localStorage.setItem('poleia_token', 'search-fixture');
          window.S = (await import('/static/js/megaron/state.js')).State;
          S.WORLD_ID = 'world-fixture';
          S.provinceData = [{id:'city', own:true, name:'Petras', q:0, r:0}];
          window.search = await import('/static/js/megaron/ui/search.js');
          window.closeSearch = search.closeSearch;
          window.centreOn = search.centreOn;
          window.openDrawer = async name => {
            window.openedDrawer = name;
            document.getElementById('drawer-' + name).classList.add('open');
            if (name === 'gossip') await (await import('/static/js/megaron/ui/drawers/gossip.js')).loadGossipDrawer();
            else await (await import('/static/js/megaron/ui/drawers/diplomacy.js')).loadDiplomacyDrawer();
          };
          search.toggleSearch();
        }''')
        inp = page.locator('#search-input')
        inp.fill('COPPER')
        letter = page.locator('[data-search-drawer="diplomacy"]')
        expect(letter).to_contain_text('Knossos')
        assert not page.evaluate('window.attacked || false')
        page.locator('#search-overlay').screenshot(path=str(OUT / 'letters-desktop.png'))
        inp.press('Enter')
        expect(page.locator('#search-overlay')).not_to_have_class(re.compile(r'open'))
        expect(page.locator('#dtab-threads')).to_contain_text('Knossos')
        page.evaluate('search.toggleSearch()')
        expect(inp).to_have_value('')
        inp.fill('tin')
        rumour = page.locator('[data-search-drawer="gossip"]')
        expect(rumour).to_contain_text('Tin is scarce')
        inp.press('ArrowDown')
        expect(rumour).to_have_class(re.compile(r'focused'))
        inp.press('Enter')
        expect(page.locator('#gossip-body')).to_contain_text('Tin is scarce')
        page.evaluate('search.toggleSearch()')
        inp.fill('Petras')
        expect(page.locator('#search-results')).to_contain_text('Your Cities')
        page.evaluate('search.closeSearch()')
        # Partial failure must not hide the successful source or claim no results.
        page.route('**/messengers/inbox', lambda route: route.fulfill(status=403, body='{}'))
        page.evaluate('search.toggleSearch()')
        inp.fill('tin')
        expect(page.locator('#search-results')).to_contain_text('Could not load: Received letters')
        expect(rumour).to_contain_text('Tin is scarce')
        rumour.click()
        assert page.evaluate('window.openedDrawer') == 'gossip'
        page.unroute('**/messengers/inbox')
        # Resolve an old request after a new session: it cannot overwrite new data.
        page.evaluate('''() => {
          window.originalFetch = window.fetch;
          window.pendingSearch = [];
          window.fetch = () => new Promise(resolve => pendingSearch.push(resolve));
          search.toggleSearch();
          search.closeSearch();
          search.toggleSearch();
          const response = value => new Response(JSON.stringify(value), {headers: {'Content-Type':'application/json'}});
          pendingSearch[2](response([{from_name:'New city',message:'fresh copper'}]));
          pendingSearch[3](response([]));
        }''')
        inp.fill('copper')
        expect(letter).to_contain_text('fresh copper')
        page.evaluate('''() => {
          pendingSearch[0](new Response(JSON.stringify([{message:'stale copper'}])));
          pendingSearch[1](new Response('[]'));
          window.fetch = originalFetch;
        }''')
        page.wait_for_timeout(100)
        expect(letter).to_contain_text('fresh copper')
        expect(page.locator('#search-results')).not_to_contain_text('stale copper')
        # A fresh open re-reads the API; responsive existing rows still fit.
        page.evaluate('search.closeSearch(); search.toggleSearch()')
        page.set_viewport_size({'width': 390, 'height': 844})
        inp.fill('copper')
        expect(letter).to_contain_text('Knossos')
        page.locator('#search-overlay').screenshot(path=str(OUT / 'letters-mobile.png'))
        assert page.locator('#search-results').evaluate('(el) => el.scrollWidth <= el.clientWidth')
        assert not errors, errors
        browser.close()
    (OUT / 'proof.json').write_text(json.dumps({'result':'PASS', 'browser_errors':errors, 'authenticated_requests':len(requests), 'scope':'Actual DOM/search/drawer modules; explicit API fixtures, not live gameplay'}, indent=2))
    print('PASS: text, escaping, Enter, arrows, click, drawers, refresh, partial failure, stale response, mobile width')
finally:
    server.shutdown()
