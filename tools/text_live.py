#!/usr/bin/env python3
"""Real drawer text, help links and inline feedback.
Usage: python3 tools/text_live.py OUT BUILD_COMMIT baseline|after [WEB_DIR]
OUT contains a freshly built temenos. baseline uses archived unchanged WEB_DIR.
Every arm creates fresh PG16/Redis and uses ordinary player APIs and web clicks;
no SQL fixtures. Only own processes/containers are removed.
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

ROOT = Path(__file__).resolve().parents[1]
OUT = Path(sys.argv[1]).resolve()
EXPECTED = sys.argv[2]
MODE = sys.argv[3] if len(sys.argv) > 3 else "after"
assert MODE in ("baseline", "after")
OUT.mkdir(parents=True, exist_ok=True)
PREFIX = 'megaron-text-' + secrets.token_hex(4)
containers, errors = [], []
proc = None
browser = playwright = None


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
    pg = launch('pg', 'postgres:16-alpine', ['-e', 'POSTGRES_PASSWORD=simple-recall-all-pw', '-e', 'POSTGRES_DB=simple-recall-all'])
    redis = launch('redis', 'redis:7-alpine', [])
    for _ in range(60):
        if subprocess.run(['docker', 'exec', containers[0], 'pg_isready', '-h', '127.0.0.1', '-U', 'postgres', '-q'], capture_output=True).returncode == 0:
            break
        time.sleep(.25)
    with socket.socket() as sock:
        sock.bind(('127.0.0.1', 0))
        gameport = sock.getsockname()[1]
    base = f'http://127.0.0.1:{gameport}'
    env = {'HOME': os.environ['HOME'], 'PATH': os.environ['PATH'],
           'DATABASE_URL': f'postgres://postgres:simple-recall-all-pw@127.0.0.1:{pg}/simple-recall-all?sslmode=disable',
           'REDIS_URL': f'127.0.0.1:{redis}', 'JWT_SECRET': secrets.token_urlsafe(40),
           'PORT': str(gameport), 'TICK_SECONDS': '6', 'MAP_WIDTH': '56', 'MAP_HEIGHT': '40',
           'WORLD_NAME': 'Expedition proof', 'POLEIA_WORLD_START_WANAXES': '1',
           'STATIC_DIR': str(Path(sys.argv[4]).resolve()/'static') if len(sys.argv)>4 else str(ROOT/'web/static'), 'TEMPLATE_DIR': str(Path(sys.argv[4]).resolve()/'templates') if len(sys.argv)>4 else str(ROOT/'web/templates'),
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
    assert health['commit'] == EXPECTED and health['migration'] == 160, health
    worlds = api('/api/v1/worlds')
    world = (worlds if isinstance(worlds, list) else worlds['worlds'])[0]['id']
    token = api('/api/v1/auth/register', 'POST', {'username': 'simple-recall-all'+secrets.token_hex(4), 'password': secrets.token_urlsafe(32)})['access_token']
    worldpath = '/api/v1/worlds/' + world
    joined = api(worldpath+'/join', 'POST', {}, token)
    founded = api(worldpath+'/founding/settle', 'POST', {'name': 'Nostos'}, token)
    assert founded.get('settlement_id'), founded
    data=api(worldpath+'/units',token=token)
    chosen=[u for u in data['units'] if u.get('deployable') and u['category']=='land' and u['status']=='garrison'][:2]
    assert len(chosen)==2,data
    provinces=api(worldpath+'/provinces',token=token)
    provinces=provinces if isinstance(provinces,list) else provinces['provinces']
    city=next(p for p in provinces if p.get('settlement_id')==founded['settlement_id'] or p.get('id')==founded.get('province_id'))
    homeq,homer=city.get('q',city.get('map_q')),city.get('r',city.get('map_r'))
    tiles=api(worldpath+'/map',token=token)
    tiles=tiles if isinstance(tiles,list) else tiles['tiles']
    from playwright.sync_api import sync_playwright, expect
    playwright=sync_playwright().start();browser=playwright.chromium.launch()
    page=browser.new_page(viewport={'width':1280,'height':900})
    dialogs=[]
    page.on('pageerror',lambda error:errors.append(str(error)))
    page.on('dialog',lambda dialog:(dialogs.append(dialog.message),dialog.dismiss()))
    page.goto(base+'/',wait_until='domcontentloaded')
    page.evaluate('t=>localStorage.setItem("poleia_token",t)',token)
    page.context.add_cookies([{'name':'poleia_token','value':token,'url':base}])
    page.goto(base+'/play',wait_until='networkidle')
    page.wait_for_function('window.openDrawer !== undefined')
    def open_drawer(drawer):
        page.evaluate('d=>{document.querySelectorAll(".drawer.open").forEach(e=>window.closeDrawer(e.id.slice(7)));window.openDrawer(d);}',drawer)
    for drawer in ['city','war','diplomacy','economy','kult','notif']:
        open_drawer(drawer);page.wait_for_timeout(500)
        for label,width,height in [('desktop',1280,900),('mobile',390,844)]:
            page.set_viewport_size({'width':width,'height':height});page.wait_for_timeout(100)
            page.locator('#drawer-'+drawer).screenshot(path=str(OUT/(drawer+'-'+label+'.png')))
        if MODE=='after':
            brief=page.locator('#lb-'+drawer)
            assert len(brief.inner_text())<150,brief.inner_text()
            brief.get_by_role('button',name='Read the Codex',exact=True).click()
            page.wait_for_function('document.getElementById("codex-panel").classList.contains("open")')
            assert page.locator('#codex-title').inner_text()==next(a['title'] for a in api('/static/codex/index.json')['articles'] if a.get('drawer')==drawer)
            page.evaluate('window.closeCodex()')
    receipts={}
    if MODE=='after':
        open_drawer('city');page.wait_for_selector('#city-gubbe-grid svg')
        page.locator('#drawer-city button[data-tab="byggnader"]').click()
        with page.expect_response(lambda r:'/build-queue/' in r.url and r.request.method=='DELETE') as pending:
            page.evaluate('(id)=>window.cancelBuild(id,"00000000-0000-0000-0000-000000000001")',city['id'])
        receipts['cancel_refusal']={'status':pending.value.status,'body':pending.value.json()}
        assert pending.value.status>=400
        line=page.locator('#city-bld-sec [data-inline-result]');line.wait_for();assert line.inner_text()==pending.value.json()['error']
        line.scroll_into_view_if_needed();page.locator('#drawer-city').screenshot(path=str(OUT/'cancel-inline-mobile.png'))
        open_drawer('kult');page.wait_for_timeout(300)
        with page.expect_response(lambda r:r.url.endswith('/rite') and r.request.method=='POST') as pending:
            page.evaluate('window.okRite("unknown-proof-prayer")')
        receipts['rite_refusal']={'status':pending.value.status,'body':pending.value.json()}
        assert pending.value.status>=400
        line=page.locator('#kult-body [data-inline-result]');line.wait_for();assert line.inner_text()==pending.value.json()['error']
        line.scroll_into_view_if_needed();page.locator('#drawer-kult').screenshot(path=str(OUT/'rite-inline-mobile.png'))
        # Explicit synthetic replay of historic kinds no longer emitted by the server.
        # No DB writes: only the archive endpoint is replayed, real renderer/click/filter.
        filters=[]
        def historic(route):
            url=route.request.url;filters.append(url)
            kind='SitosFundLow' if 'SitosFundLow' in url and 'exclude=' not in url else 'SitosIntervention'
            rows=[] if 'exclude=' in url else [{'kind':kind,'body':{},'level':4,'created_at':'2026-10-08T00:00:00Z','read_at':'read'}]*3
            route.fulfill(json={'notifications':rows})
        page.route('**/notifications?*',historic)
        open_drawer('notif');page.get_by_text('three food support reports — show reports',exact=True).wait_for()
        page.locator('#drawer-notif').screenshot(path=str(OUT/'historic-groups-mobile.png'))
        page.get_by_text('three food support reports — show reports',exact=True).click()
        page.get_by_text('Food support was given to the city',exact=True).first.wait_for()
        assert 'SitosIntervention' not in page.locator('#notif-body').inner_text()
        page.locator('#notif-body').get_by_text('All notifications',exact=True).click();page.get_by_text('three food reserve warnings — show reports',exact=True).wait_for()
        receipts['historic_replay']={'synthetic':True,'filters':filters}
        assert 'World speed' not in page.locator('#notif-body').inner_text()
    assert not dialogs,dialogs
    assert not errors,errors
    proof={'health':health,'mode':MODE,'browser_errors':errors,'browser_dialogs':dialogs,'receipts':receipts,'sql_mutations':False}
    (OUT/'proof.json').write_text(json.dumps(proof,indent=2)+'\n');print(json.dumps(proof))
except Exception:
    if browser is not None:
        try:page.screenshot(path=str(OUT/'failure.png'))
        except Exception:pass
    raise
finally:
    if browser is not None:browser.close()
    if playwright is not None:playwright.stop()
    if proc is not None:proc.terminate();proc.wait(timeout=20)
    for name in containers:subprocess.run(['docker','rm','-fv',name],capture_output=True)
