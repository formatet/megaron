#!/usr/bin/env python3
"""Firefox proof of real join/inspect controllers with scripted HTTP responses.

Checks request headers, error rendering, success redirect and accepted/refused
event boundaries. No live game mutations or outbound analytics calls.
"""
import functools
import http.server
import json
from pathlib import Path
import re
import threading
from playwright.sync_api import sync_playwright, expect

ROOT = Path(__file__).resolve().parents[1]
OUT = ROOT / 'docs/reviews/umami-verb/browser.json'
WORLD = '12345678-1234-1234-1234-123456789abc'
SETTLEMENT = '22345678-1234-1234-1234-123456789abc'
DEST = '32345678-1234-1234-1234-123456789abc'
HARNESS = '''<script>
localStorage.removeItem('umami-proof-events');
window.umami={track(name,props){const a=JSON.parse(localStorage.getItem('umami-proof-events')||'[]');a.push({name,props});localStorage.setItem('umami-proof-events',JSON.stringify(a));}};
</script>'''


class Handler(http.server.SimpleHTTPRequestHandler):
    def log_message(self, *args):
        pass


def events(page):
    return page.evaluate("JSON.parse(localStorage.getItem('umami-proof-events')||'[]')")


def main():
    server = http.server.ThreadingHTTPServer(('127.0.0.1', 0), functools.partial(Handler, directory=str(ROOT / 'web')))
    threading.Thread(target=server.serve_forever, daemon=True).start()
    base = f'http://127.0.0.1:{server.server_port}'
    join = (ROOT / 'web/templates/join.html').read_text().replace('{{.WorldID}}', WORLD)
    join = re.sub(r'{{.*?}}', '', join)
    results = []
    try:
        with sync_playwright() as pw:
            browser = pw.firefox.launch()
            try:
                for token, status in [(True, 201), (False, 201), (True, 409)]:
                    page = browser.new_page()
                    errors, requests = [], []
                    page.on('pageerror', lambda err: errors.append(str(err)))
                    init = "localStorage.setItem('poleia_token','fixture-token');" if token else "localStorage.removeItem('poleia_token');"
                    page.route('**/fixture-join', lambda route: route.fulfill(content_type='text/html', body=HARNESS + '<script>' + init + '</script>' + join))
                    page.route('**/world/*/map', lambda route: route.fulfill(content_type='text/html', body='<p>Joined world</p>'))

                    def join_api(route):
                        req = route.request
                        assert req.method == 'POST'
                        assert req.headers.get('content-type') == 'application/json'
                        assert req.headers.get('authorization') == ('Bearer fixture-token' if token else None)
                        assert not req.post_data
                        requests.append('join')
                        route.fulfill(status=status, content_type='application/json', body='{}' if status < 400 else '{"error":"World full"}')

                    page.route('**/api/v1/**', join_api)
                    page.goto(base + '/fixture-join')
                    page.wait_for_function("typeof window.joinWorld === 'function'")
                    page.locator('.btn-join-world').click()
                    if status < 400:
                        page.wait_for_url(base + f'/world/{WORLD}/map')
                        assert events(page) == [{'name': 'world_joined'}]
                    else:
                        expect(page.locator('#join-result')).to_have_text('World full')
                        page.wait_for_function("JSON.parse(localStorage.getItem('umami-proof-events')||'[]').length===1")
                        assert events(page) == [{'name': 'verb_refused', 'props': {'verb': 'join'}}]
                    assert requests == ['join'] and not errors, (requests, errors)
                    results.append({'controller': 'join', 'token': token, 'status': status, 'headers': 'identical', 'redirect_or_error': 'PASS', 'events': events(page)})
                    page.close()

                for origin, status in [(None, 201), (SETTLEMENT, 201), (None, 422)]:
                    page = browser.new_page()
                    errors, writes = [], []
                    page.on('pageerror', lambda err: errors.append(str(err)))
                    page.route('**/fixture-inspect', lambda route: route.fulfill(content_type='text/html', body=HARNESS + '''<script>localStorage.setItem('poleia_token','fixture-token');</script><div id="map-root"><canvas id="hex-canvas"></canvas></div><div id="tile-tooltip"></div><textarea id="ip-msg-text">A private letter</textarea><span id="ip-msg-err"></span>'''))

                    def inspect_api(route):
                        req = route.request
                        if req.method == 'GET':
                            route.fulfill(content_type='application/json', body='[]')
                            return
                        path = f'settlements/{origin}/messengers' if origin else 'founding/messengers'
                        assert req.url == base + f'/api/v1/worlds/{WORLD}/{path}'
                        assert req.method == 'POST'
                        assert req.headers.get('authorization') == 'Bearer fixture-token'
                        assert req.headers.get('content-type') == 'application/json'
                        assert json.loads(req.post_data) == {'destination_id': DEST, 'message': 'A private letter'}
                        writes.append(path)
                        route.fulfill(status=status, content_type='application/json', body='{}' if status < 400 else '{"error":"insufficient_goods"}')

                    page.route('**/api/v1/**', inspect_api)
                    page.goto(base + '/fixture-inspect')
                    page.evaluate('''async ([world,origin,dest])=>{
                        const {State}=await import('/static/js/megaron/state.js');
                        State.WORLD_ID=world;State.MY_SETTLEMENT_ID=origin;
                        const {sendMessengerFromInspect}=await import('/static/js/megaron/render/map.js');
                        await sendMessengerFromInspect(dest);
                    }''', [WORLD, origin, DEST])
                    if status < 400:
                        expect(page.locator('#ip-msg-text')).to_have_value('')
                        expect(page.locator('#ip-msg-err')).to_have_text('Messenger sent.')
                        assert events(page) == [{'name': 'messenger_sent'}]
                    else:
                        expect(page.locator('#ip-msg-text')).to_have_value('A private letter')
                        expect(page.locator('#ip-msg-err')).to_have_text('insufficient_goods')
                        page.wait_for_function("JSON.parse(localStorage.getItem('umami-proof-events')||'[]').length===1")
                        assert events(page) == [{'name': 'verb_refused', 'props': {'verb': 'message', 'reason_code': 'insufficient_goods'}}]
                    assert len(writes) == 1 and not errors, (writes, errors)
                    results.append({'controller': 'inspect', 'origin': 'city' if origin else 'host', 'status': status, 'headers_and_body': 'identical', 'error_or_clear': 'PASS', 'events': events(page)})
                    page.close()
                OUT.write_text(json.dumps(results, indent=2) + '\n')
                print(f'Firefox: {len(results)} real-controller scenarios PASS (scripted HTTP, no live game writes)')
            finally:
                browser.close()
    finally:
        server.shutdown()


if __name__ == '__main__':
    main()
