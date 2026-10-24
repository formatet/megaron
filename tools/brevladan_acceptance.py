#!/usr/bin/env python3
"""Owned fresh runtime, ordinary founding, real HTTP letters/offers and browser actions.
Usage: python3 tools/brevladan_acceptance.py OUT [BUILD_COMMIT]
Explicit fixtures are saved. Never touches shared or production services.
"""
from pathlib import Path
import hashlib
import json
import os
import secrets
import socket
import subprocess
import sys
import tempfile
import time
import urllib.request
import uuid
from playwright.sync_api import sync_playwright

ROOT = Path(os.environ.get('BREVLADAN_SOURCE', Path(__file__).resolve().parents[1])).resolve()
BASELINE = os.environ.get('BREVLADAN_BASELINE') == '1'


def main():
    out = Path(sys.argv[1]).resolve()
    out.mkdir(parents=True, exist_ok=True)
    prefix = 'megaron-brevladan-' + secrets.token_hex(4)
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
        with tempfile.TemporaryDirectory(prefix='brevladan-proof-') as directory:
            tmp = Path(directory)
            clean = {k: os.environ[k] for k in ('HOME', 'PATH')}
            clean['CGO_ENABLED'] = '0'
            commit = sys.argv[2] if len(sys.argv) > 2 else run('git', '-C', str(ROOT), 'rev-parse', '--short=7', 'HEAD') + '+brevladan'
            def source_hashes():
                files = [p for d in ('server/api/handlers', 'server/cmd/server', 'server/internal/messenger', 'server/cmd/keryx', 'web/static/js', 'web/templates', 'web/static/codex')
                         for p in (ROOT / d).rglob('*') if p.is_file() and p.suffix in ('.go', '.js', '.html', '.md') and not p.name.endswith('_test.go')]
                files.append(ROOT / 'web/static/megaron.css')
                return {str(p.relative_to(ROOT)): hashlib.sha256(p.read_bytes()).hexdigest() for p in sorted(files)}
            hashes = source_hashes()
            run('go', 'build', '-ldflags', '-X main.buildCommit=' + commit, '-o', str(tmp / 'temenos'), './cmd/server', cwd=ROOT / 'server', env=clean)
            run('go', 'build', '-o', str(tmp / 'keryx'), './cmd/keryx', cwd=ROOT / 'server', env=clean)
            print('BUILD_COMPLETE', commit, flush=True)
            pg = launch('pg', 'postgres:16-alpine', ['-e', 'POSTGRES_PASSWORD=brevladan', '-e', 'POSTGRES_DB=brevladan'])
            redis = launch('redis', 'redis:7-alpine', [])
            for _ in range(100):
                if subprocess.run(['docker', 'exec', containers[0], 'pg_isready', '-h', '127.0.0.1', '-U', 'postgres', '-q'], capture_output=True).returncode == 0:
                    break
                time.sleep(.2)
            with socket.socket() as sock:
                sock.bind(('127.0.0.1', 0))
                port = sock.getsockname()[1]
            base = f'http://127.0.0.1:{port}'
            env = {**clean, 'DATABASE_URL': f'postgres://postgres:brevladan@localhost:{pg}/brevladan?sslmode=disable',
                   'REDIS_URL': f'localhost:{redis}', 'JWT_SECRET': secrets.token_urlsafe(40), 'PORT': str(port), 'TICK_SECONDS': '3600',
                   'MAP_WIDTH': '56', 'MAP_HEIGHT': '40', 'WORLD_NAME': 'Letters proof', 'POLEIA_WORLD_START_WANAXES': '1',
                   'STATIC_DIR': str(ROOT / 'web/static'), 'TEMPLATE_DIR': str(ROOT / 'web/templates'),
                   'CHRONICLE_DIR': str(tmp / 'chronicles'), 'REPORTS_DIR': str(tmp / 'reports')}
            assert not (ROOT / 'server/.env').exists(), 'Do not inherit server .env'
            proc = subprocess.Popen([str(tmp / 'temenos')], cwd=ROOT / 'server', env=env,
                                    stdout=(out / 'server.log').open('w'), stderr=subprocess.STDOUT)

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
                        raise RuntimeError('server exited; see server.log')
                    time.sleep(.5)
            else:
                raise RuntimeError('server timeout')
            latest = max(int(p.name.split('_')[0]) for p in (ROOT / 'server/db/migrations').glob('*.up.sql'))
            assert health['commit'] == commit and health['migration'] == latest, health
            world = api('/api/v1/worlds')[0]['id']
            wp = '/api/v1/worlds/' + world
            fixtures = []

            def sql(q, fixture=False):
                if fixture:
                    fixtures.append(q)
                    (out / 'fixture.sql').write_text('\n\n'.join(fixtures) + '\n')
                return run('docker', 'exec', containers[0], 'psql', '-U', 'postgres', '-d', 'brevladan', '-At', '-v', 'ON_ERROR_STOP=1', '-c', q)

            def actor():
                token = api('/api/v1/auth/register', 'POST', {'username': 'Letters-' + secrets.token_hex(4), 'password': secrets.token_urlsafe(30)})['access_token']
                api(wp + '/join', 'POST', {}, token)
                host = next(u for u in api(wp + '/units', token=token)['units'] if u['type'] == 'nomadic_host')
                return {'token': token, 'host': host, 'pid': api('/api/v1/auth/me', token=token)['id']}

            me = actor()
            tiles = json.loads(sql(f"SELECT json_agg(t) FROM (SELECT q,r,terrain FROM map_tiles WHERE world_id='{world}')t"))
            dry = {(t['q'], t['r']) for t in tiles if t['terrain'] not in ('coastal_sea', 'deep_sea', 'river', 'mountain_limestone', 'mountain_red')}
            unseen = set(dry)
            connected = set()
            while unseen:
                seed = unseen.pop()
                component, pending = {seed}, [seed]
                while pending:
                    q, r = pending.pop()
                    for dq, dr in ((1, 0), (1, -1), (0, -1), (-1, 0), (-1, 1), (0, 1)):
                        point = (q + dq, r + dr)
                        if point in unseen:
                            unseen.remove(point)
                            component.add(point)
                            pending.append(point)
                if len(component) > len(connected):
                    connected = component
            selected = [me] if (me['host']['q'], me['host']['r']) in connected else []
            for _ in range(60):
                if len(selected) == 4:
                    break
                a = actor()
                if (a['host']['q'], a['host']['r']) in connected:
                    selected.append(a)
            else:
                raise AssertionError('not enough ordinary actors on largest dry component')
            me, *others = selected
            myrine, dorian, thestor = others
            actors = [me, myrine, dorian, thestor]
            for who, name, city in zip(actors, ('Leandros', 'Myrine', 'Dorian', 'Thestor'), ('Nostos', 'Kyme', 'Aulis', 'Pylos')):
                who['name'] = name
                who['sid'] = api(wp + '/founding/settle', 'POST', {'name': city}, who['token'])['settlement_id']
                who['province'] = next(p for p in api(wp + '/provinces', token=who['token']) if p.get('settlement_id') == who['sid'])
                sql(f"UPDATE players SET wanax_name='{name}' WHERE id='{who['pid']}';", fixture=True)
            for a in actors:
                for b in actors:
                    if a is not b:
                        sql(f"INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('{world}','{a['pid']}','{b['province']['id']}') ON CONFLICT DO NOTHING;", fixture=True)
            ids = ','.join("'" + a['sid'] + "'" for a in actors)
            sql(f"UPDATE worlds SET current_tick=100,last_tick_at=now() WHERE id='{world}'; UPDATE settlement_goods SET amount=10000,rate=0,calc_tick=100 WHERE settlement_id IN({ids}) AND good_key IN('grain','silver','copper');", fixture=True)

            def send(sender, recipient, text, offer=None):
                data = {'destination_id': recipient['sid'], 'message': text}
                if offer:
                    data['trade_offer'] = offer
                return api(wp + '/settlements/' + sender['sid'] + '/messengers', 'POST', data, sender['token'])

            first = send(myrine, me, 'The winter crossing is clear.\nWill you join us at the harbour?')
            offer = send(myrine, me, 'Two copper for five silver.', {'kind': 'sell', 'offer_good': 'copper', 'offer_qty': 2, 'want_silver': 5})
            completed = send(thestor, me, 'Our envoys have returned safely.')
            outgoing = send(me, dorian, 'Can you spare grain for the next voyage?')
            if not BASELINE:
                assert api(wp + '/letters', token=me['token'])['items'], 'sender must see departure'
                before = api(wp + '/letters', token=me['token'])
                assert not any(i['counterpart_id'] == myrine['pid'] for i in before['items']), 'incoming departure leaked'
            delivered = [first['id'], offer['id'], completed['id']]
            delivered_sql = ','.join("'" + x + "'" for x in delivered)
            sql(f"UPDATE scheduled_events SET due_tick=100,process_after=now() WHERE world_id='{world}' AND event_type='MessengerArrival' AND payload->>'messenger_id' IN({delivered_sql});", fixture=True)
            deadline = time.monotonic() + 40
            while time.monotonic() < deadline:
                if int(sql(f"SELECT count(*) FROM messengers WHERE id IN({delivered_sql}) AND status='delivered'")) == 3:
                    break
                time.sleep(.2)
            else:
                raise AssertionError('actual Arrival worker did not deliver')
            config = tmp / 'config.json'
            with os.fdopen(os.open(config, os.O_WRONLY | os.O_CREAT, 0o600), 'w') as f:
                json.dump({'server': base, 'token': me['token'], 'world_id': world, 'province_id': me['province']['id']}, f)

            def cli(*args):
                return run(str(tmp / 'keryx'), *args, env={**clean, 'POLEIA_CONFIG': str(config)})

            if BASELINE:
                (out / 'queue-before.json').write_text(json.dumps(api(wp + '/messengers/queue', token=me['token']), indent=2))
                (out / 'cli-before.txt').write_text(cli('inbox', '--queue'))
                errors = []
                with sync_playwright() as pw:
                    browser = pw.firefox.launch()
                    for label, width, height in [('desktop', 1280, 900), ('mobile', 390, 844)]:
                        ctx = browser.new_context(viewport={'width': width, 'height': height})
                        ctx.add_cookies([{'name': 'poleia_token', 'value': me['token'], 'url': base}])
                        ctx.add_init_script('localStorage.setItem("poleia_token",' + json.dumps(me['token']) + ')')
                        page = ctx.new_page(); page.on('pageerror', lambda e: errors.append(str(e)))
                        page.goto(base + '/world/' + world + '/map', wait_until='networkidle')
                        for brief in page.locator('.lb-dismiss').all():
                            if brief.is_visible(): brief.click()
                        page.locator('#trig-diplomacy').click()
                        page.locator('#dtab-queue [data-letter]').first.wait_for()
                        page.screenshot(path=str(out / (label + '-before.png')))
                        (out / (label + '-before.txt')).write_text(page.locator('#diplomacy-body').inner_text())
                        assert len(page.locator('#diplomacy-body .dtab').all_text_contents()) == 5
                        ctx.close()
                    browser.close()
                assert not errors, errors
                (out / 'proof.json').write_text(json.dumps({'health': health, 'world': world, 'map_seed': sql(f"SELECT map_seed FROM worlds WHERE id='{world}'"), 'source_hashes': hashes, 'page_errors': errors, 'fixture': 'normal register/join/found; explicit names, scouted contacts and stocks; real HTTP sends; advanced three arrival timers and actual worker delivery', 'result': 'BASELINE'}, indent=2))
                print('BASELINE', health, 'five diplomacy tabs at1280/390', flush=True)
                return
            (out / 'cli-completed-reply.json').write_text(cli('reply', '--id', completed['id'], '--text', 'Welcome home. We shall meet at Pylos.', '--json'))
            letters = api(wp + '/letters', token=me['token'])
            (out / 'letters-before.json').write_text(json.dumps(letters, indent=2))
            summary = {i['counterpart_id']: i for i in letters['items']}
            assert summary[myrine['pid']]['state'] == 'waiting_you', summary
            assert summary[dorian['pid']]['state'] == 'waiting_them', summary
            assert summary[thestor['pid']]['state'] == 'done', summary
            assert letters['items'][0]['counterpart_id'] == myrine['pid'], letters
            (out / 'cli-list.txt').write_text(cli('letters'))
            (out / 'cli-thread.txt').write_text(cli('letters', 'Myrine'))
            (out / 'cli-thread.json').write_text(cli('letters', myrine['pid'], '--json'))
            errors, posts, screenshots = [], [], []
            with sync_playwright() as pw:
                browser = pw.firefox.launch()
                contexts = []
                for label, width, height, codes in [('desktop', 1280, 900, ('A1', 'A2', 'A3', 'A4')), ('mobile', 390, 844, ('B1', 'B2', 'B3', 'B4'))]:
                    ctx = browser.new_context(viewport={'width': width, 'height': height}, has_touch=width == 390)
                    contexts.append(ctx)
                    ctx.add_cookies([{'name': 'poleia_token', 'value': me['token'], 'url': base}])
                    ctx.add_init_script('localStorage.setItem("poleia_token",' + json.dumps(me['token']) + ')')
                    page = ctx.new_page()
                    page.on('pageerror', lambda e: errors.append(str(e)))
                    page.on('request', lambda r: posts.append({'method': r.method, 'path': r.url.split(base)[-1]}) if r.method == 'POST' else None)
                    page.goto(base + '/world/' + world + '/map', wait_until='networkidle')
                    for brief in page.locator('.lb-dismiss').all():
                        if brief.is_visible():
                            brief.click()
                    page.locator('#trig-diplomacy').click()
                    root = page.locator('#dtab-letters')
                    root.locator('[data-person="' + myrine['pid'] + '"]').wait_for()
                    assert page.locator('#diplomacy-body .dtab').all_text_contents() == ['Letters', 'Runners']
                    assert 'Waiting on you' in root.inner_text() and 'Waiting on them' in root.inner_text() and 'Done' in root.inner_text()
                    assert root.locator('[data-person]').first.get_attribute('data-person') == myrine['pid']
                    for forbidden in ('Personal archive', 'Refresh', 'legacy', 'Private note', 'Memory with this Wanax'):
                        assert forbidden not in page.locator('#diplomacy-body').inner_text(), forbidden
                    filename = codes[0] + '-' + label + '-letters.png'
                    page.screenshot(path=str(out / filename)); screenshots.append({'code': codes[0], 'file': filename, 'description': f'Letters, {width}px'})
                    root.locator('[data-person="' + myrine['pid'] + '"]').click()
                    root.locator('[data-letter="' + offer['id'] + '"]').wait_for()
                    assert root.locator('[data-letter]').last.get_attribute('open') is not None
                    assert root.evaluate('(e)=>e.scrollWidth<=e.clientWidth'), 'horizontal thread overflow'
                    filename = codes[1] + '-' + label + '-thread.png'
                    page.screenshot(path=str(out / filename)); screenshots.append({'code': codes[1], 'file': filename, 'description': f'Exchange and actions, {width}px'})
                    root.locator('[data-letters-back]').click()
                    root.locator('[data-write-list]').click()
                    root.locator('[data-write="' + myrine['sid'] + '"]').wait_for()
                    filename = codes[2] + '-' + label + '-write.png'
                    page.screenshot(path=str(out / filename)); screenshots.append({'code': codes[2], 'file': filename, 'description': f'Write to reachable Wanax, {width}px'})
                    page.locator('[data-tab="runners"]').click()
                    page.locator('#dtab-runners .runner-list').wait_for()
                    filename = codes[3] + '-' + label + '-runners.png'
                    page.screenshot(path=str(out / filename)); screenshots.append({'code': codes[3], 'file': filename, 'description': f'Runners, {width}px'})
                    assert page.locator('#diplomacy-body').evaluate('(e)=>e.scrollWidth<=e.clientWidth'), 'horizontal drawer overflow'
                    (out / (label + '-text.txt')).write_text(page.locator('#diplomacy-body').inner_text())
                page = contexts[0].pages[0]
                page.locator('[data-tab="letters"]').click()
                root = page.locator('#dtab-letters')
                root.locator('[data-person="' + myrine['pid'] + '"]').wait_for()
                root.locator('[data-person="' + myrine['pid'] + '"]').click()
                trade = root.locator('#dip-trade-' + offer['id'])
                with page.expect_response(lambda r: r.request.method == 'POST' and r.url.endswith('/' + offer['id'] + '/trade-accept')) as response:
                    trade.get_by_role('button', name='Accept', exact=True).click()
                assert response.value.status == 200, response.value.text()
                root.get_by_text('Agreed: 2 copper for 5 silver', exact=False).wait_for()
                page.screenshot(path=str(out / 'C1-desktop-agreed.png'))
                screenshots.append({'code': 'C1', 'file': 'C1-desktop-agreed.png', 'description': 'Accepted offer remains in the exchange'})
                reply = root.locator('#dip-reply-' + first['id'])
                reply.fill('We shall meet you at the harbour.')
                with page.expect_response(lambda r: r.request.method == 'POST' and r.url.endswith('/' + first['id'] + '/reply')) as response:
                    root.get_by_role('button', name='Send with their runner', exact=False).click()
                assert response.value.status == 200, response.value.text()
                after_reply = api(wp + '/letters/' + myrine['pid'], token=me['token'])
                (out / 'thread-after.json').write_text(json.dumps(after_reply, indent=2))
                assert int(sql(f"SELECT count(*) FROM messengers WHERE world_id='{world}'")) == 4, 'reply created an extra bearer'
                ground = json.loads(sql(f"SELECT row_to_json(t) FROM(SELECT id,status,reply_text FROM messengers WHERE id='{first['id']}')t"))
                assert ground['status'] == 'returning' and ground['reply_text'] == 'We shall meet you at the harbour.', ground
                gift_id = str(uuid.uuid4())
                gift_payload = {'transport_id': gift_id, 'sender_id': myrine['pid'], 'recipient_id': me['pid'],
                                'sender_name': 'Myrine', 'recipient_name': 'Leandros', 'origin_id': myrine['sid'],
                                'origin_name': 'SECRET GIFT ORIGIN', 'destination_id': me['sid'], 'destination_name': 'Nostos',
                                'good_key': 'copper', 'quantity': 1.5, 'credited_quantity': 1.5, 'lost_quantity': 0}
                quoted_payload = json.dumps(gift_payload).replace("'", "''")
                sql(f"INSERT INTO events(stream_id,stream_type,event_type,payload,world_id) VALUES('{me['sid']}','province','GiftDelivered','{quoted_payload}','{world}');", fixture=True)
                gift_thread = api(wp + '/letters/' + myrine['pid'], token=me['token'])
                gift_history = api(wp + '/gifts', token=me['token'])
                assert 'SECRET GIFT ORIGIN' not in json.dumps(gift_thread) + json.dumps(gift_history)
                gift_row = next(m for m in gift_thread['items'] if m['id'] == gift_id)
                assert gift_row['goods'] == {'copper': 1.5}, gift_row
                (out / 'gift-thread.json').write_text(json.dumps(gift_thread, indent=2))
                (out / 'gift-history.json').write_text(json.dumps(gift_history, indent=2))
                page.reload(wait_until='networkidle')
                page.locator('#trig-diplomacy').click()
                root = page.locator('#dtab-letters')
                root.locator('[data-person="' + myrine['pid'] + '"]').click()
                root.locator('[data-letter="' + gift_id + '"]').wait_for()
                assert 'Gift received: 1.5 copper' in root.inner_text(), root.inner_text()
                assert 'SECRET GIFT ORIGIN' not in root.inner_text()
                page.screenshot(path=str(out / 'C2-desktop-gift.png'))
                screenshots.append({'code': 'C2', 'file': 'C2-desktop-gift.png', 'description': 'Recorded gift outcome, person name and exact goods; origin hidden'})
                assert not errors, errors
                for ctx in contexts:
                    ctx.close()
                browser.close()
            (out / 'cli-after.txt').write_text(cli('letters', 'Myrine'))
            (out / 'screenshots.json').write_text(json.dumps(screenshots, indent=2))
            assert source_hashes() == hashes, 'runtime sources changed while proof ran; repeat with stable sources'
            (out / 'source-sha256.json').write_text(json.dumps(hashes, indent=2))
            proof = {'health': health, 'world': world, 'map_seed': sql(f"SELECT map_seed FROM worlds WHERE id='{world}'"),
                     'actors': [{k: v for k, v in a.items() if k != 'token'} for a in actors], 'letters': {'free': first, 'offer': offer, 'completed': completed, 'outgoing': outgoing},
                     'source_hashes': hashes, 'posts': posts, 'page_errors': errors, 'reply_ground': ground,
                     'fixture': 'normal register/join/found; explicit human Wanax names, mutual scouted contacts and stocks; real HTTP sends; three arrival timers advanced to current100 and processed by actual worker; fourth remains outbound; actual CLI reply and native browser accept/reply; explicit recorded GiftDelivered outcome fixture only for projection/privacy/decimal display, no claim of actual gift delivery', 'result': 'PASS'}
            (out / 'proof.json').write_text(json.dumps(proof, indent=2))
            print('PASS', health, 'Firefox1280/390 list/thread/Write/Runners, native acceptance+same-runner reply, actual Keryx', flush=True)
    finally:
        if proc is not None:
            proc.terminate()
            proc.wait(timeout=20)
        for name in containers:
            subprocess.run(['docker', 'rm', '-fv', name], capture_output=True)


if __name__ == '__main__':
    main()
