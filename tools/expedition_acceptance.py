"""Actual expedition UI + authenticated preview and order requests, with explicit API fixtures.
Run: python3 tools/expedition_acceptance.py [output-dir]
"""
import functools
import http.server
import json
from pathlib import Path
import re
import sys
import threading
from urllib.parse import urlparse, parse_qs
from playwright.sync_api import sync_playwright, expect

ROOT = Path(__file__).resolve().parents[1]
OUT = Path(sys.argv[1] if len(sys.argv) > 1 else '/tmp/megaron-expedition-browser')
OUT.mkdir(parents=True, exist_ok=True)
class Handler(http.server.SimpleHTTPRequestHandler):
    def log_message(self, *_): pass
server = http.server.ThreadingHTTPServer(('127.0.0.1', 0), functools.partial(Handler, directory=str(ROOT / 'web')))
threading.Thread(target=server.serve_forever, daemon=True).start()
url = f'http://127.0.0.1:{server.server_port}'
html = re.sub(r'<script\b[^>]*>.*?</script>', '', (ROOT/'web/static/map.html').read_text(), flags=re.S)
units = [dict(id=i, type='spearman', display_name=n, settlement_id='city', category='land', status='garrison', deployable=True) for i,n in [('fast','First Spearmen'),('slow','Second Spearmen')]]
units.append(dict(id='expedition',type='spearman',display_name='Third Spearmen',category='land',status='marching',deployable=True,current_q=2,current_r=1,target_q=0,target_r=0,expedition={'area_q':9,'area_r':2,'length_ticks':12,'turn_tick':108,'home_by_tick':114,'homeward':True,'turn_reason':'area_known'}))
requests, errors, orders = [], [], []
try:
    with sync_playwright() as p:
        browser = p.chromium.launch()
        page = browser.new_page(viewport={'width':1280,'height':900})
        page.on('pageerror', lambda e: errors.append(str(e)))
        page.route('**/fixture', lambda route: route.fulfill(content_type='text/html', body=html))
        def api(route):
            req = route.request
            assert req.headers.get('authorization') == 'Bearer preview-fixture'
            assert req.method == 'GET' or req.url.endswith('/march'), 'Unexpected mutation'
            requests.append(req.url)
            path = urlparse(req.url).path
            if path.endswith('/march'):
                body = req.post_data_json
                assert body['intent'] == 'explore' and body['ticks'] == 12, body
                orders.append(body)
                data = {'status':'order_dispatched','courier_arrives_at':'2099-01-01T00:00:00Z'}
            elif path.endswith('/march-preview'):
                query = parse_qs(urlparse(req.url).query)
                if query.get('target_q') == ['9']:
                    data = {'available':False,'reason':'courier_required'}
                elif query.get('intent') == ['explore']:
                    data = {'available':False,'reason':'unknown_terrain'}
                else:
                    ticks = 8 if '/slow/' in path else 2
                    data = {'available':True,'arrival_tick':100+ticks,'duration_ticks':ticks,'arrives_at_utc':'2099-01-01T00:00:00Z'}
            elif path == '/api/v1/units': data = []
            elif path.endswith('/units'): data = {'units':units,'expedition_rules':{'min_ticks':4,'max_ticks':30,'default_ticks':10,'area_radius':5}}
            elif '/provinces/' in path: data = {'settlement':{'army':{},'buildings':[]}}
            elif path.endswith('/retreat-default'): data = {'by_loyalty':True}
            else: data = []
            route.fulfill(content_type='application/json', body=json.dumps(data))
        page.route('**/api/**', api)
        page.goto(url+'/fixture', wait_until='domcontentloaded')
        page.evaluate('''async () => {
          localStorage.setItem('poleia_token','preview-fixture');
          window.S = (await import('/static/js/megaron/state.js')).State;
          S.WORLD_ID='world-fixture'; S.unitsData=[]; S.CURRENT_TICK=100; S.TICK_SECONDS=3600; S.TICK_ANCHOR_MS=Date.now();
          S.provinceData=[{id:'city',settlement_id:'city',name:'Knossos',own:true,q:0,r:0}];
          window.march = await import('/static/js/megaron/ui/marchctx.js');
          window.onExploreToggle=march.onExploreToggle; window.onColonizeToggle=march.onColonizeToggle;
          await march.openMarchCtx({q:3,r:2,known:true,isSea:false,name:'Known plains',isSettlement:true},150,150);
        }''')
        page.locator('#mg-0').fill('2')
        eta = page.locator('#mctx-eta')
        expect(eta).to_contain_text('First Spearmen: Estimated arrival')
        expect(eta).to_contain_text('2 game days')
        expect(eta).to_contain_text('8 game days')
        page.locator('#march-ctx').screenshot(path=str(OUT/'map-desktop.png'))
        page.locator('#mg-0').fill('0')
        expect(eta).not_to_be_visible()
        page.evaluate("march.openMarchCtx({q:4,r:2,known:false,isSea:false,name:'Unknown land'},150,150)")
        page.locator('#mg-0').fill('1')
        expect(page.locator('#mctx-ticks')).to_have_value('10')
        assert page.locator('#mctx-ticks').get_attribute('min') == '4'
        assert page.locator('#mctx-ticks').get_attribute('max') == '30'
        page.locator('#mctx-ticks').fill('12')
        expect(page.locator('#mctx-mission')).to_contain_text('for 12 game days')
        expect(page.locator('#mctx-mission')).to_contain_text('within 5 hexes')
        expect(eta).to_contain_text('unexplored terrain')
        assert any('ticks=12' in r and 'intent=explore' in r for r in requests)
        page.locator('#march-ctx').screenshot(path=str(OUT/'expedition-map-desktop.png'))
        page.set_viewport_size({'width':390,'height':844})
        page.locator('#march-ctx').screenshot(path=str(OUT/'expedition-map-mobile.png'))
        assert page.locator('#march-ctx').evaluate('(e)=>e.scrollWidth<=e.clientWidth')
        page.set_viewport_size({'width':1280,'height':900})
        page.locator('#mg-1').fill('1')
        expect(page.locator('#mctx-mission')).to_contain_text('marching units receive a redirect and end their expedition')
        page.locator('#mg-0').fill('0')
        expect(page.locator('#mctx-expedition')).not_to_be_visible()
        expect(eta).to_contain_text('Runner must deliver')
        page.locator('#mg-1').fill('0')
        page.locator('#mg-0').fill('1')
        page.evaluate('march.sendMarch()')
        assert len(orders) == 1
        
        page.evaluate("march.closeMarchCtx(); march.openMarchCtx({q:9,r:2,known:true,isSea:false,name:'Plains',isSettlement:true},150,150)")
        page.locator('#mg-0').fill('1')
        expect(eta).to_contain_text('Runner must deliver')
        # Import actual war renderer, render army tab, use its actual input bindings.
        page.evaluate('''async () => {
          march.closeMarchCtx();
          window.war=await import('/static/js/megaron/ui/drawers/war.js');
          window.wmpLandToggle=war.wmpLandToggle;
          document.getElementById('drawer-war').classList.add('open');
          S.unitsData=(await (await fetch('/api/v1/worlds/world-fixture/units',{headers:{Authorization:'Bearer preview-fixture'}})).json()).units;
          await war.loadWarDrawer();
          war.unitMarch('fast');
        }''')
        expect(page.locator('#ucard-expedition')).to_contain_text('returning home — area explored; home by game day 114')
        page.locator('#ucard-expedition').screenshot(path=str(OUT/'expedition-mission.png'))
        page.locator('#wmp-q').fill('3')
        expect(page.locator('#wmp-eta')).to_contain_text('Estimated arrival')
        page.locator('#wmp-explore-chk').check()
        page.locator('#wmp-ticks').fill('12')
        expect(page.locator('#wmp-mission')).to_contain_text('for 12 game days')
        expect(page.locator('#wmp-eta')).to_contain_text('unexplored terrain')
        page.locator('#drawer-war').screenshot(path=str(OUT/'expedition-war-desktop.png'))
        page.set_viewport_size({'width':390,'height':844})
        page.locator('#drawer-war').screenshot(path=str(OUT/'expedition-war-mobile.png'))
        assert page.locator('#war-march-panel').evaluate('(e)=>e.scrollWidth<=e.clientWidth')
        page.set_viewport_size({'width':1280,'height':900})
        page.evaluate('war.unitMarchSend()')
        assert len(orders) == 2
        page.evaluate("war.unitMarch('fast')")
        page.locator('#wmp-explore-chk').uncheck()
        page.locator('#wmp-q').fill('9')
        expect(page.locator('#wmp-eta')).to_contain_text('Runner must deliver')
        page.locator('#wmp-q').fill('')
        expect(page.locator('#wmp-eta')).to_be_empty()
        page.evaluate('war.closeMarchPanel()')
        page.set_viewport_size({'width':390,'height':844})
        page.evaluate("document.getElementById('drawer-war').classList.remove('open'); march.openMarchCtx({q:3,r:2,known:true,isSea:false,name:'Known plains',isSettlement:true},100,100)")
        page.locator('#mg-0').fill('2')
        expect(eta).to_contain_text('8 game days')
        assert page.locator('#march-ctx').evaluate('(e)=>e.scrollWidth<=e.clientWidth')
        page.locator('#march-ctx').screenshot(path=str(OUT/'map-mobile.png'))
        assert not errors, errors
        browser.close()
    (OUT/'proof.json').write_text(json.dumps({'result':'PASS','requests':len(requests),'orders':orders,'errors':errors,'scope':'Actual web modules; explicit API fixtures, not live gameplay'},indent=2))
    print('PASS: actual map + War modules, server rules, 12-day summary, preview ticks, order ticks, desktop + mobile; explicit API fixtures')
finally:
    server.shutdown()
