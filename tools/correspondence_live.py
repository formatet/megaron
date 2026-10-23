#!/usr/bin/env python3
"""K1: fresh PG/Redis, normal register/join/found, labelled delivered-letter
fixtures, actual Keryx and Firefox queue/reply/decline/paging at 1280 and 390.
Usage: python3 tools/correspondence_live.py OUT [EXPECTED_BUILD]
Builds current sources in a clean environment. Never touches shared services.
The browser_queue helper can also be imported by another owned-runtime proof.
"""
import hashlib
import json
import os
from pathlib import Path
import secrets
import socket
import subprocess
import sys
import tempfile
import time
import urllib.request
import uuid
from playwright.sync_api import sync_playwright

ROOT = Path(__file__).resolve().parents[1]


def browser_queue(base, world, token, out, expected):
    """Real HTTP/browser only. expected contains reply/poor/expired letter IDs."""
    out = Path(out)
    out.mkdir(parents=True, exist_ok=True)
    errors, posts, captures, history_reads = [], [], [], []
    with sync_playwright() as pw:
        browser = pw.firefox.launch()
        for label, width, height in [('desktop', 1280, 900), ('mobile', 390, 844)]:
            reads_before = len([p for p in posts if p.endswith('/read')])
            history_before = len(history_reads)
            ctx = browser.new_context(viewport={'width': width, 'height': height}, has_touch=label == 'mobile')
            ctx.add_cookies([{'name': 'poleia_token', 'value': token, 'url': base}])
            ctx.add_init_script('localStorage.setItem("poleia_token",' + json.dumps(token) + ')')
            page = ctx.new_page()
            page.on('pageerror', lambda e: errors.append(str(e)))
            def record_request(request):
                if request.method == 'POST':
                    posts.append(request.url.split(base)[-1])
                if request.url.endswith('/messengers/inbox?history=true'):
                    history_reads.append(request.url.split(base)[-1])
            page.on('request', record_request)
            page.goto(base + '/world/' + world + '/map', wait_until='networkidle')
            for brief in page.locator('.lb-dismiss').all():
                if brief.is_visible():
                    brief.click()
            page.locator('#trig-diplomacy').click()
            root = page.locator('#dtab-queue')
            root.locator('[data-letter]').first.wait_for()
            assert root.is_visible(), 'Queue must open by default'
            assert len(history_reads) == history_before, 'Default Queue fetched hidden legacy history'
            assert 'Groups apply to this page' in root.inner_text()
            assert len([p for p in posts if p.endswith('/read')]) == reads_before, 'Listing acknowledged letters'
            poor = root.locator('[data-letter="' + expected['poor'] + '"]')
            expired = root.locator('[data-letter="' + expected['expired'] + '"]')
            assert poor.count() == expired.count() == 1
            if label == 'desktop':
                assert poor.locator('[data-queue-action="trade_decline"]').count() == 1
                assert poor.locator('[data-queue-action="trade_accept"]').count() == 0
                assert 'insufficient stock' in poor.inner_text()
            assert 'Expired without answer' in expired.inner_text()
            assert expired.locator('[data-queue-action]').count() == 0
            root.locator('[data-letter]').first.scroll_into_view_if_needed()
            page.screenshot(path=str(out / (label + '-queue.png')))
            expired.scroll_into_view_if_needed()
            page.screenshot(path=str(out / (label + '-missed.png')))
            assert root.evaluate('(el)=>el.scrollWidth<=el.clientWidth'), 'Queue overflows horizontally'
            before_ids = root.locator('[data-letter]').evaluate_all('(els)=>els.map(el=>el.dataset.letter)')
            assert len(before_ids) == 30
            root.locator('[data-queue-next]').click()
            page.wait_for_function("document.querySelectorAll('#dtab-queue [data-letter]').length === 5")
            next_ids = root.locator('[data-letter]').evaluate_all('(els)=>els.map(el=>el.dataset.letter)')
            assert len(set(before_ids + next_ids)) == 35, 'Lost or duplicate page row'
            root.locator('[data-queue-refresh]').click()
            root.locator('[data-letter="' + expected['reply'] + '"]').wait_for()
            if label == 'desktop':
                letter = root.locator('[data-letter="' + expected['reply'] + '"]')
                with page.expect_response(lambda r: r.request.method == 'POST' and r.url.endswith('/' + expected['reply'] + '/read')) as read:
                    letter.locator('summary').click()
                assert read.value.status == 200
                assert len([p for p in posts if p.endswith('/read')]) == 1
                letter.locator('[data-queue-reply]').fill('Return this answer on your own road.')
                with page.expect_response(lambda r: r.request.method == 'POST' and r.url.endswith('/' + expected['reply'] + '/reply')) as reply:
                    letter.locator('[data-queue-action="reply"]').click()
                assert reply.value.status == 200, reply.value.text()
                letter.get_by_text('Reply dispatched.', exact=True).wait_for()
                page.screenshot(path=str(out / 'desktop-reply.png'))
                with page.expect_response(lambda r: r.request.method == 'POST' and r.url.endswith('/' + expected['poor'] + '/trade-decline')) as decline:
                    root.locator('[data-letter="' + expected['poor'] + '"] [data-queue-action="trade_decline"]').click()
                assert decline.value.status == 200, decline.value.text()
                root.get_by_text('Offer declined.', exact=True).wait_for()
            # The legacy unbounded history is fetched only on explicit selection.
            page.locator('[data-tab="threads"]').click()
            page.locator('#dtab-threads .dip-thread').first.wait_for()
            assert len(history_reads) == history_before + 1, 'Explicit Correspondence click did not load history exactly once'
            page.locator('[data-tab="queue"]').click()
            assert root.is_visible()
            captures.append({'view': label, 'width': width, 'height': height, 'first_page': before_ids, 'next_page': next_ids})
            ctx.close()
        browser.close()
    assert not errors, errors
    result = {'captures': captures, 'errors': errors, 'posts': posts, 'history_reads': history_reads}
    (out / 'browser.json').write_text(json.dumps(result, indent=2))
    return result


def main():
    out = Path(sys.argv[1]).resolve()
    out.mkdir(parents=True, exist_ok=True)
    prefix = 'megaron-correspondence-' + secrets.token_hex(4)
    containers, proc = [], None

    def run(*args, **kw):
        return subprocess.check_output(args, text=True, **kw).strip()

    def launch(kind, image, args):
        name = prefix + '-' + kind
        internal = '5432' if kind == 'pg' else '6379'
        run('docker', 'run', '-d', '--rm', '--name', name, '-p', '127.0.0.1::' + internal, *args, image)
        containers.append(name)
        ports = json.loads(run('docker', 'inspect', name))[0]['NetworkSettings']['Ports']
        return int(ports[internal + '/tcp'][0]['HostPort'])

    try:
        with tempfile.TemporaryDirectory(prefix='correspondence-live-') as tmpdir:
            tmp = Path(tmpdir)
            clean = {k: os.environ[k] for k in ('HOME', 'PATH')}
            clean['CGO_ENABLED'] = '0'
            commit = sys.argv[2] if len(sys.argv) > 2 else run('git', '-C', str(ROOT), 'rev-parse', '--short=7', 'HEAD') + '+k1'
            hashes = {str(p.relative_to(ROOT)): hashlib.sha256(p.read_bytes()).hexdigest()
                      for directory in ('server/api/handlers', 'server/cmd/keryx', 'web/static/js/megaron/ui/drawers')
                      for p in (ROOT / directory).glob('*') if p.is_file() and ('messenger' in p.name or 'correspondence' in p.name or 'diplomacy' in p.name)}
            run('go', 'build', '-ldflags', '-X main.buildCommit=' + commit, '-o', str(tmp / 'temenos'), './cmd/server', cwd=ROOT / 'server', env=clean)
            run('go', 'build', '-o', str(tmp / 'keryx'), './cmd/keryx', cwd=ROOT / 'server', env=clean)
            print('BUILD_COMPLETE', commit, flush=True)
            pg = launch('pg', 'postgres:16-alpine', ['-e', 'POSTGRES_PASSWORD=k1-proof', '-e', 'POSTGRES_DB=k1_proof'])
            redis = launch('redis', 'redis:7-alpine', [])
            for _ in range(80):
                if subprocess.run(['docker', 'exec', containers[0], 'pg_isready', '-h', '127.0.0.1', '-U', 'postgres', '-q'], capture_output=True).returncode == 0:
                    break
                time.sleep(.25)
            with socket.socket() as sock:
                sock.bind(('127.0.0.1', 0))
                port = sock.getsockname()[1]
            base = f'http://127.0.0.1:{port}'
            env = {**clean, 'DATABASE_URL': f'postgres://postgres:k1-proof@localhost:{pg}/k1_proof?sslmode=disable', 'REDIS_URL': f'localhost:{redis}',
                   'JWT_SECRET': secrets.token_urlsafe(40), 'PORT': str(port), 'TICK_SECONDS': '3600', 'MAP_WIDTH': '56', 'MAP_HEIGHT': '40',
                   'WORLD_NAME': 'Correspondence K1 proof', 'POLEIA_WORLD_START_WANAXES': '1', 'STATIC_DIR': str(ROOT / 'web/static'),
                   'TEMPLATE_DIR': str(ROOT / 'web/templates'), 'CHRONICLE_DIR': str(tmp / 'chronicles'), 'REPORTS_DIR': str(tmp / 'reports')}
            assert not (ROOT / 'server/.env').exists()
            proc = subprocess.Popen([str(tmp / 'temenos')], cwd=ROOT / 'server', env=env, stdout=(out / 'server.log').open('w'), stderr=subprocess.STDOUT)

            def api(path, method='GET', data=None, token=None):
                req = urllib.request.Request(base + path, method=method, data=None if data is None else json.dumps(data).encode(),
                                            headers={'Content-Type': 'application/json', **({'Authorization': 'Bearer ' + token} if token else {})})
                return json.load(urllib.request.urlopen(req, timeout=30))

            for _ in range(120):
                try:
                    health = api('/healthz')
                    break
                except Exception:
                    if proc.poll() is not None:
                        raise RuntimeError('Server exited; see server.log')
                    time.sleep(.5)
            else:
                raise RuntimeError('Server timeout')
            latest = max(int(p.name.split('_')[0]) for p in (ROOT / 'server/db/migrations').glob('*.up.sql'))
            assert health['commit'] == commit and health['migration'] == latest, health
            world = api('/api/v1/worlds')[0]['id']
            wp = '/api/v1/worlds/' + world

            def sql(q):
                return run('docker', 'exec', containers[0], 'psql', '-U', 'postgres', '-d', 'k1_proof', '-At', '-v', 'ON_ERROR_STOP=1', '-c', q)

            def actor():
                token = api('/api/v1/auth/register', 'POST', {'username': 'Correspondence-' + secrets.token_hex(4), 'password': secrets.token_urlsafe(30)})['access_token']
                api(wp + '/join', 'POST', {}, token)
                host = next(u for u in api(wp + '/units', token=token)['units'] if u['type'] == 'nomadic_host')
                land = sql(f"SELECT landmass_id FROM map_tiles WHERE world_id='{world}' AND q={host['q']} AND r={host['r']}")
                return {'token': token, 'host': host, 'landmass': land, 'pid': api('/api/v1/auth/me', token=token)['id']}

            recipient = actor()
            for _ in range(40):
                sender = actor()
                if sender['landmass'] == recipient['landmass']:
                    break
            else:
                raise AssertionError('No ordinary pair on same connected landmass')
            for who, name in [(recipient, 'K1 Nostos'), (sender, 'K1 Kyme')]:
                who['sid'] = api(wp + '/founding/settle', 'POST', {'name': name}, who['token'])['settlement_id']
                who['province'] = next(p for p in api(wp + '/provinces', token=who['token']) if p.get('settlement_id') == who['sid'])
            expected = {kind: str(uuid.uuid4()) for kind in ('reply', 'poor', 'expired')}
            ids = [expected['reply'], expected['poor'], expected['expired']] + [str(uuid.uuid4()) for _ in range(32)]
            # This is an explicit post-delivery queue fixture, not a claim of
            # natural contact/travel. Population/positions were normally founded.
            fixture = [f"UPDATE settlement_goods SET amount=CASE WHEN good_key='grain' THEN 10000 ELSE 0 END, rate=0,calc_tick=current_world_tick() WHERE settlement_id='{recipient['sid']}';",
                       f"UPDATE settlement_goods SET amount=100,rate=0,calc_tick=current_world_tick() WHERE settlement_id='{sender['sid']}' AND good_key='copper';"]
            for n, ident in enumerate(ids):
                trade = "NULL"
                deadline = "NULL"
                if ident in (expected['poor'], expected['expired']):
                    trade = "'{\"kind\":\"sell\",\"offer_good\":\"copper\",\"offer_qty\":2,\"want_silver\":5,\"status\":\"pending\"}'::jsonb"
                    deadline = "now()+interval '1 hour'" if ident == expected['poor'] else "now()-interval '1 minute'"
                fixture.append(f"INSERT INTO messengers(id,world_id,sender_id,origin_id,destination_id,message_text,trade_offer,status,hex_q,hex_r,sent_at,arrives_at,expires_at) VALUES('{ident}','{world}','{sender['pid']}','{sender['sid']}','{recipient['sid']}','K1 delivered fixture {n+1}',{trade},'delivered',{recipient['province']['q']},{recipient['province']['r']},now()-interval '{n+1} seconds',now()-interval '1 second',{deadline});")
                fixture.append(f"INSERT INTO events(stream_id,stream_type,event_type,payload,world_id) VALUES('{recipient['sid']}','province','MessengerArrived','{{\"messenger_id\":\"{ident}\"}}','{world}');")
            script = 'BEGIN;\n' + '\n'.join(fixture) + '\nCOMMIT;'
            sql(script)
            (out / 'fixture.sql').write_text(script)
            (out / 'fixture.json').write_text(json.dumps({'expected': expected, 'ids': ids, 'sender': {k: v for k, v in sender.items() if k != 'token'}, 'recipient': {k: v for k, v in recipient.items() if k != 'token'}}, indent=2))
            first = api(wp + '/messengers/queue?limit=30', token=recipient['token'])
            assert len(first['items']) == 30 and first.get('next_cursor'), first
            (out / 'queue-before.json').write_text(json.dumps(first, indent=2))
            assert int(sql(f"SELECT count(*) FROM messengers WHERE world_id='{world}' AND read_at IS NOT NULL")) == 0
            config = tmp / 'config.json'
            with os.fdopen(os.open(config, os.O_WRONLY | os.O_CREAT, 0o600), 'w') as f:
                json.dump({'server': base, 'token': recipient['token'], 'world_id': world, 'province_id': recipient['province']['id']}, f)

            def cli(*args):
                return run(str(tmp / 'keryx'), *args, env={**clean, 'POLEIA_CONFIG': str(config)})

            (out / 'queue-cli.txt').write_text(cli('inbox', '--queue'))
            raw = cli('inbox', '--queue', '--json')
            parsed = json.loads(raw)
            assert len(parsed['items']) == 30
            second = json.loads(cli('inbox', '--queue', '--json', '--cursor', parsed['next_cursor']))
            assert len(second['items']) == 5 and len(set(m['id'] for m in parsed['items'] + second['items'])) == 35
            (out / 'queue-cli.json').write_text(raw)
            (out / 'queue-cli-next.json').write_text(json.dumps(second, indent=2))
            assert int(sql(f"SELECT count(*) FROM messengers WHERE world_id='{world}' AND read_at IS NOT NULL")) == 0, 'CLI listing wrote reads'
            browser = browser_queue(base, world, recipient['token'], out, expected)
            after = json.loads(sql(f"SELECT json_agg(t) FROM (SELECT id,status,reply_text,read_at,trade_offer FROM messengers WHERE id IN('{expected['reply']}','{expected['poor']}','{expected['expired']}') ORDER BY id)t"))
            by_id = {m['id']: m for m in after}
            assert by_id[expected['reply']]['status'] == 'returning' and by_id[expected['reply']]['reply_text'] == 'Return this answer on your own road.'
            assert by_id[expected['poor']]['trade_offer']['status'] == 'declined' and by_id[expected['poor']]['status'] == 'returning'
            assert by_id[expected['expired']]['trade_offer']['status'] == 'pending' and by_id[expected['expired']]['read_at'] is None, 'GET mutated expired escrow or read unopened offer'
            assert int(sql(f"SELECT count(*) FROM messengers WHERE world_id='{world}' AND read_at IS NOT NULL")) == 1
            assert float(sql(f"SELECT amount FROM settlement_goods WHERE settlement_id='{sender['sid']}' AND good_key='copper'")) == 102, 'Decline did not refund exactly once'
            (out / 'ground-after.json').write_text(json.dumps(after, indent=2))
            final = api(wp + '/messengers/queue?limit=100', token=recipient['token'])
            assert len(final['items']) == 35
            for ident in expected.values():
                assert next(m for m in final['items'] if m['id'] == ident)['queue_group'] == 'completed'
            (out / 'queue-after.json').write_text(json.dumps(final, indent=2))
            (out / 'proof.json').write_text(json.dumps({'health': health, 'world': world, 'source_hashes': hashes, 'browser': browser,
                                                       'fixtures': 'normal register/join/found; explicit 35 delivered letters + arrival outcomes + insolvent recipient stock before real queue/actions', 'result': 'PASS'}, indent=2))
            print('PASS', health, '35 letters, real CLI, native reply/decline, Firefox1280/390, exact read/refund checks')
    finally:
        if proc is not None:
            proc.terminate()
            proc.wait(timeout=20)
        for name in containers:
            subprocess.run(['docker', 'rm', '-fv', name], capture_output=True)


if __name__ == '__main__':
    main()
