#!/usr/bin/env python3
"""Real Production cards, placement, daily history and livestock slaughter.
Usage: python3 tools/production_live.py OUT BUILD_COMMIT baseline|after [WEB_DIR]
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
PREFIX = 'megaron-production-cards-' + secrets.token_hex(4)
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
    page.on('pageerror',lambda error:errors.append(str(error)))
    page.goto(base+'/',wait_until='domcontentloaded')
    page.evaluate('t=>localStorage.setItem("poleia_token",t)',token)
    page.context.add_cookies([{'name':'poleia_token','value':token,'url':base}])
    page.goto(base+'/play',wait_until='networkidle')
    page.wait_for_function('window.openDrawer !== undefined')
    page.evaluate("window.openDrawer('city')")
    page.wait_for_selector('#city-gubbe-grid svg')
    for brief in page.locator('.lb-dismiss').all():
        if brief.is_visible():brief.click()
    for label,width,height in [('desktop',1280,900),('mobile',390,844)]:
        page.set_viewport_size({'width':width,'height':height});page.wait_for_timeout(150)
        page.locator('#city-body').evaluate('(e)=>e.scrollTop=0')
        page.locator('#drawer-city').screenshot(path=str(OUT/('production-'+label+'.png')))
        assert page.locator('#city-body').evaluate('(e)=>e.scrollWidth<=e.clientWidth'),'city overflow'
        if MODE!='baseline':
            page.locator('#city-sitos-sec').scroll_into_view_if_needed()
            page.locator('#drawer-city').screenshot(path=str(OUT/('production-food-'+label+'.png')))
            details=page.locator('#city-more');assert not details.evaluate('(e)=>e.open')
            details.locator('summary').click();details.scroll_into_view_if_needed();page.locator('#drawer-city').screenshot(path=str(OUT/('production-more-'+label+'.png')));details.locator('summary').click()
    page.set_viewport_size({'width':1280,'height':900})
    options=api(worldpath+'/provinces/'+city['id']+'/placement-options',token=token)
    # Free one placed worker, then place the same good on the same hex again.
    target=next((h,g) for h in options['hexes'] for g in h['goods'] if g.get('placed_ordinals'))
    h,g=target
    def row():return page.locator('.gubbe-good-row[data-good="'+g['good_key']+'"]')
    page.locator('.gubbe-hex-g[data-ordinal="'+str(h['hex_ordinal'])+'"]').click()
    with page.expect_response(lambda r:'/placements/' in r.url and r.request.method=='DELETE') as pending:
        row().locator('[data-verb="unplace1"]').click()
    deletion=pending.value;assert deletion.status==200,deletion.status
    page.wait_for_timeout(300)
    with page.expect_response(lambda r:r.url.endswith('/placements') and r.request.method=='POST') as pending:
        row().locator('[data-verb="place1"]').click()
    placement=pending.value;assert placement.status==201,placement.status
    if MODE!='baseline':page.locator('#city-more summary').click()
    page.get_by_role('button',name='Show recent ticks' if MODE=='baseline' else 'Show recent days',exact=True).click()
    page.wait_for_timeout(300)
    assert page.locator('#city-ticklog-sec').inner_text(), 'history remains reachable'
    page.locator('#drawer-city').screenshot(path=str(OUT/'history-desktop.png'))
    before_slaughter=api(worldpath+'/provinces/'+city['id'],token=token)['settlement']
    with page.expect_response(lambda r:r.url.endswith('/slaughter-livestock') and r.request.method=='POST') as pending:
        page.locator('button[onclick="slaughterLivestock(\''+city['id']+'\')"]').click()
    slaughter=pending.value;assert slaughter.status==200,slaughter.status
    assert slaughter.json()['population']==before_slaughter['population']+10,slaughter.json()
    page.wait_for_selector('#city-gubbe-grid svg')
    page.evaluate("window.openDrawer('economy')");page.wait_for_timeout(500)
    page.locator('#drawer-economy').screenshot(path=str(OUT/'economy-desktop.png'))
    page.set_viewport_size({'width':390,'height':844});page.wait_for_timeout(150)
    page.locator('#drawer-economy').screenshot(path=str(OUT/'economy-mobile.png'))
    assert page.locator('#economy-body').evaluate('(e)=>e.scrollWidth<=e.clientWidth'),'economy overflow'
    if MODE!='baseline':
        assert '/tick' not in page.locator('#drawer-economy').inner_text()
    assert not errors,errors
    proof={'health':health,'mode':MODE,'placement_delete_status':deletion.status,'placement_post_status':placement.status,'placement_post_body':placement.request.post_data_json,'slaughter_receipt':slaughter.json(),'options':options,'final_placements':api(worldpath+'/provinces/'+city['id']+'/placements',token=token),'browser_errors':errors,'sql_mutations':False}
    (OUT/'proof.json').write_text(json.dumps(proof,indent=2)+'\n');print(json.dumps({'health':health,'mode':MODE,'placement_round_trip':True}))
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
