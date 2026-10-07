#!/usr/bin/env python3
"""Real expedition via register/join/found, web or CLI, return and report.
Usage: python3 tools/expedition_live.py OUT BUILD_COMMIT web|cli
OUT contains freshly built temenos and keryx. No SQL fixtures or inherited game
configuration; only own temporary containers/process/private config are removed.
"""
import json
import os
from pathlib import Path
import secrets
import socket
import subprocess
import sys
import time
import urllib.error
import urllib.request
from playwright.sync_api import sync_playwright, expect

ROOT = Path(__file__).resolve().parents[1]
OUT = Path(sys.argv[1]).resolve()
EXPECTED, MODE = sys.argv[2:4]
assert MODE in ('web', 'cli')
OUT.mkdir(parents=True, exist_ok=True)
PREFIX = 'megaron-expedition-' + secrets.token_hex(4)
containers, errors = [], []
proc = None


def command(*args):
    return subprocess.check_output(args, text=True).strip()


def launch(service, image, args):
    name = PREFIX + '-' + service
    internal = '5432' if service == 'pg' else '6379'
    command('docker', 'run', '-d', '--rm', '--name', name,
            '-p', '127.0.0.1::' + internal, *args, image)
    containers.append(name)
    bindings = json.loads(command('docker', 'inspect', name))[0]['NetworkSettings']['Ports']
    return int(next(iter(bindings.values()))[0]['HostPort'])


try:
    pg = launch('pg', 'postgres:16-alpine', ['-e', 'POSTGRES_PASSWORD=expedition-pw', '-e', 'POSTGRES_DB=expedition'])
    redis = launch('redis', 'redis:7-alpine', [])
    for _ in range(60):
        if subprocess.run(['docker', 'exec', containers[0], 'pg_isready', '-U', 'postgres', '-q'], capture_output=True).returncode == 0:
            break
        time.sleep(.25)
    with socket.socket() as sock:
        sock.bind(('127.0.0.1', 0))
        gameport = sock.getsockname()[1]
    base = f'http://127.0.0.1:{gameport}'
    env = {'HOME': os.environ['HOME'], 'PATH': os.environ['PATH'],
           'DATABASE_URL': f'postgres://postgres:expedition-pw@127.0.0.1:{pg}/expedition?sslmode=disable',
           'REDIS_URL': f'127.0.0.1:{redis}', 'JWT_SECRET': secrets.token_urlsafe(40),
           'PORT': str(gameport), 'TICK_SECONDS': '2', 'MAP_WIDTH': '56', 'MAP_HEIGHT': '40',
           'WORLD_NAME': 'Expedition proof', 'POLEIA_WORLD_START_WANAXES': '1',
           'STATIC_DIR': str(ROOT/'web/static'), 'TEMPLATE_DIR': str(ROOT/'web/templates'),
           'CHRONICLE_DIR': str(OUT/'chronicles'), 'REPORTS_DIR': str(OUT/'reports')}
    assert not (ROOT/'server/.env').exists(), 'Proof must not load a game .env'

    def api(path, method='GET', data=None, token=None):
        request = urllib.request.Request(base + path, method=method,
            data=None if data is None else json.dumps(data).encode(),
            headers={'Content-Type': 'application/json', **({'Authorization': 'Bearer ' + token} if token else {})})
        with urllib.request.urlopen(request, timeout=20) as response:
            return json.load(response)

    log = (OUT/'server.log').open('w')
    proc = subprocess.Popen([str(OUT/'temenos')], cwd=ROOT/'server', env=env, stdout=log, stderr=log)
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
    assert health['commit'] == EXPECTED and health['migration'] == 159, health
    worlds = api('/api/v1/worlds')
    world = (worlds if isinstance(worlds, list) else worlds['worlds'])[0]['id']
    token = api('/api/v1/auth/register', 'POST', {'username': 'expedition'+secrets.token_hex(4), 'password': secrets.token_urlsafe(32)})['access_token']
    worldpath = '/api/v1/worlds/' + world
    api(worldpath+'/join', 'POST', {}, token)
    founded = api(worldpath+'/founding/settle', 'POST', {'name': 'Periplus'}, token)
    assert founded.get('settlement_id'), founded
    data = api(worldpath+'/units', token=token)
    rules = data['expedition_rules']
    units = data['units']
    (OUT/'starting-units.json').write_text(json.dumps(units, indent=2)+'\n')
    unit = next(u for u in units if u.get('deployable') and u['status'] == 'garrison' and u['category'] == 'land')
    # Use the actual owned city's public location; the centre is a nearby area.
    provinces = api(worldpath+'/provinces', token=token)
    provinces = provinces if isinstance(provinces, list) else provinces['provinces']
    city = next(p for p in provinces if p.get('settlement_id') == founded['settlement_id'] or p.get('id') == founded.get('province_id'))
    homeq, homer = city.get('q', city.get('map_q')), city.get('r', city.get('map_r'))
    tilemap = api(worldpath+'/map', token=token)
    tiles = tilemap if isinstance(tilemap, list) else tilemap['tiles']
    # Map coordinates are axial, not a guessed rectangular q/r range.
    candidates = [t for t in tiles if max(abs(t['q']-homeq), abs(t['r']-homer), abs(t['q']+t['r']-homeq-homer)) == 4]
    assert candidates, 'no nearby area centres in actual map response'
    centre = min(candidates, key=lambda t: (abs(t['r']-homer), abs(t['q']-homeq-4), t['q'], t['r']))
    q, r = centre['q'], centre['r']
    length = rules['max_ticks']
    cfg = OUT/'private-config.json'
    cfg.write_text(json.dumps({'server': base, 'token': token, 'world_id': world}))
    cfg.chmod(0o600)
    cli_env = {'HOME': os.environ['HOME'], 'PATH': os.environ['PATH'], 'POLEIA_CONFIG': str(cfg)}

    def cli(*args):
        return subprocess.check_output([str(OUT/'keryx'), *args], env=cli_env, text=True)

    # Preview must not infer the selected unseen first leg, even for a known centre.
    preview = json.loads(cli('march', '--unit', unit['id'], '--target', f'{q},{r}', '--intent', 'explore', '--ticks', str(length), '--preview', '--json'))
    assert not preview['available'] and preview['reason'] == 'unknown_terrain', preview
    assert not any(u.get('expedition') for u in api(worldpath+'/units', token=token)['units'])
    with sync_playwright() as pw:
        browser = pw.chromium.launch()
        page = browser.new_page(viewport={'width': 1280, 'height': 900})
        page.on('pageerror', lambda error: errors.append(str(error)))
        page.goto(base+'/', wait_until='domcontentloaded')
        page.evaluate('t=>localStorage.setItem("poleia_token",t)', token)
        page.context.add_cookies([{'name': 'poleia_token', 'value': token, 'url': base}])
        page.goto(base+'/play', wait_until='networkidle')
        page.wait_for_function('window.openDrawer !== undefined')
        page.evaluate('''async ({q,r})=>{
            window.expeditionMenu=await import('/static/js/megaron/ui/marchctx.js');
            await expeditionMenu.openMarchCtx({q,r,known:false,isSea:false,name:'Unknown land'},100,100);
        }''', {'q': q, 'r': r})
        index = page.evaluate('''async id=>(await import('/static/js/megaron/state.js')).State.marchCtxGroups.findIndex(g=>g.ids.includes(id))''', unit['id'])
        assert index >= 0
        page.locator('#mg-'+str(index)).fill('1')
        page.locator('#mctx-ticks').fill(str(length))
        expect(page.locator('#mctx-mission')).to_contain_text(str(length))
        expect(page.locator('#mctx-eta')).to_contain_text('unexplored terrain')
        page.locator('#march-ctx').screenshot(path=str(OUT/'order-desktop.png'))
        page.set_viewport_size({'width': 390, 'height': 844})
        page.locator('#march-ctx').screenshot(path=str(OUT/'order-mobile.png'))
        assert page.locator('#march-ctx').evaluate('(e)=>e.scrollWidth<=e.clientWidth')
        if MODE == 'web':
            with page.expect_response(lambda response: '/units/'+unit['id']+'/march' in response.url and response.request.method == 'POST') as receipt:
                page.locator('#mctx-send').click()
            actual_receipt = receipt.value.json()
            assert receipt.value.status == 202, actual_receipt
            expect(page.locator('#mctx-eta')).to_contain_text('Marching', timeout=10000)
            ordered = next(u for u in api(worldpath+'/units', token=token)['units'] if u['id'] == unit['id'])
        else:
            page.evaluate('expeditionMenu.closeMarchCtx()')
            result = json.loads(cli('march', '--unit', unit['id'], '--target', f'{q},{r}', '--intent', 'explore', '--ticks', str(length), '--json'))
            assert result['expedition']['length_ticks'] == length, result
            ordered = next(u for u in api(worldpath+'/units', token=token)['units'] if u['id'] == unit['id'])
        assert ordered['expedition']['length_ticks'] == length, ordered
        mission_cli = cli('unit', 'list')
        assert str(length) in mission_cli and 'Expedition' in mission_cli, mission_cli
        page.evaluate('expeditionMenu.closeMarchCtx()')
        page.evaluate('''async ()=>{
          window.expeditionWar=await import('/static/js/megaron/ui/drawers/war.js');
          document.getElementById('drawer-war').classList.add('open');
          await expeditionWar.loadWarDrawer();
        }''')
        page.locator('#drawer-war').screenshot(path=str(OUT/'mission-mobile.png'))
        page.set_viewport_size({'width': 1280, 'height': 900})
        page.locator('#drawer-war').screenshot(path=str(OUT/'mission-desktop.png'))
        history = []
        deadline = time.monotonic() + 110
        while time.monotonic() < deadline:
            current = next(u for u in api(worldpath+'/units', token=token)['units'] if u['id'] == unit['id'])
            state = {k: current.get(k) for k in ('status', 'q', 'r', 'target_q', 'target_r', 'arrival_tick', 'expedition')}
            if not history or history[-1] != state:
                history.append(state)
            notices = api(worldpath+'/notifications?kind=ExpeditionTurnedHome,ExpeditionReport', token=token)['notifications']
            reports = [n for n in notices if n['kind'] == 'ExpeditionReport']
            if reports:
                break
            time.sleep(.5)
        else:
            raise AssertionError({'no_homecoming_report': history})
        assert current['status'] == 'garrison' and current['settlement_id'] == founded['settlement_id'] and not current.get('expedition'), current
        turns = [n for n in notices if n['kind'] == 'ExpeditionTurnedHome']
        assert len(reports) == len(turns) == 1, notices
        report = reports[0]['body']
        assert report['ticks_out'] <= length and report['hexes_seen'] > 0, report
        report_cli = cli('notifications', '--kind', 'ExpeditionReport')
        assert 'saw' in report_cli.lower(), report_cli
        formatter = page.evaluate('''async body=>{
          const f=await import('/static/js/megaron/ui/format.js');
          return f.notifText('ExpeditionReport',body);
        }''', report)
        assert 'saw' in formatter.lower(), formatter
        assert not errors, errors
        browser.close()
    # Read-only ground proof; no SQL fixture changes.
    ground = command('docker', 'exec', containers[0], 'psql', '-U', 'postgres', '-d', 'expedition', '-tAc',
        f"SELECT json_build_object('expedition_rows',(SELECT count(*) FROM unit_expeditions),'seen_rows',(SELECT count(*) FROM unit_expedition_seen),'failed_arrivals',(SELECT count(*) FROM scheduled_events WHERE event_type='UnitArrival' AND failed_at IS NOT NULL),'dirty',(SELECT dirty FROM schema_migrations))")
    proof = {'health': health, 'mode': MODE, 'registered_joined_founded': True, 'rules': rules,
             'order': {'area_q': q, 'area_r': r, 'length_ticks': length}, 'initial_mission': ordered['expedition'],
             'history': history, 'turn': turns[0]['body'], 'report': report, 'ground': json.loads(ground),
             'preview': preview, 'cli_mission': mission_cli, 'cli_report': report_cli, 'web_report': formatter,
             'browser_errors': errors, 'scope': 'Real clean PostgreSQL16/Redis7/server, CLI, web and player APIs; no SQL fixture writes'}
    assert proof['ground']['expedition_rows'] == proof['ground']['seen_rows'] == proof['ground']['failed_arrivals'] == 0
    assert proof['ground']['dirty'] is False
    (OUT/'proof.json').write_text(json.dumps(proof, indent=2)+'\n')
    print('PASS', MODE, json.dumps(report))
finally:
    if proc is not None:
        proc.terminate()
        proc.wait(timeout=20)
    for name in containers:
        subprocess.run(['docker', 'rm', '-f', name], capture_output=True)
    if (OUT/'private-config.json').exists():
        (OUT/'private-config.json').unlink()
