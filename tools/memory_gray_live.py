#!/usr/bin/env python3
"""Real player scout and return, fresh PG16/Redis. Usage: OUT COMMIT baseline|after [WEB_DIR]. OUT contains freshly built temenos. No SQL mutations."""
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
PREFIX = 'megaron-memory-gray-' + secrets.token_hex(4)
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
    if MODE == 'naval':
        for attempt in range(20):
            spawn = joined['tile']
            seen = api(worldpath+'/map', token=token)
            seen = seen if isinstance(seen, list) else seen['tiles']
            if any(t['q'] == spawn['Q'] and t['r'] == spawn['R'] and t.get('coastal') for t in seen):
                break
            token = api('/api/v1/auth/register', 'POST', {'username': 'warship'+secrets.token_hex(4), 'password': secrets.token_urlsafe(32)})['access_token']
            joined = api(worldpath+'/join', 'POST', {}, token)
        else:raise AssertionError('no coastal spawn via player joins')
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
    nearby=[t for t in tiles if t['terrain'] not in ('fog','coastal_sea','deep_sea','river','river_ford','mountain_limestone','mountain_red') and not any(p.get('q')==t['q'] and p.get('r')==t['r'] for p in provinces) and max(abs(t['q']-homeq),abs(t['r']-homer),abs(t['q']+t['r']-homeq-homer))>=1 and max(abs(t['q']-homeq),abs(t['r']-homer),abs(t['q']+t['r']-homeq-homer))<=2]
    from playwright.sync_api import sync_playwright, expect
    import urllib.parse
    uid=chosen[0]['id']
    candidates=[t for t in tiles if t['terrain'] not in ('fog','river','river_ford','coastal_sea','deep_sea','mountain_limestone','mountain_red') and 2<=max(abs(t['q']-homeq),abs(t['r']-homer),abs(t['q']+t['r']-homeq-homer))<=4]
    candidates.sort(key=lambda t:(max(abs(t['q']-homeq),abs(t['r']-homer),abs(t['q']+t['r']-homeq-homer)),t['q'],t['r']))
    target=None
    for t in candidates:
        try:
            receipt=api(worldpath+'/units/'+uid+'/march','POST',{'target_q':t['q'],'target_r':t['r'],'intent':'explore','ticks':14},token)
        except urllib.error.HTTPError as e:
            if e.code==422:continue
            raise
        target=t;break
    assert target,'no reachable scout area via real player order'
    forecast={'available':False,'reason':'unknown_terrain (expedition intentionally has no ETA forecast)'}
    deadline=time.monotonic()+240;saw_away=False
    while time.monotonic()<deadline:
        current=next(u for u in api(worldpath+'/units',token=token)['units'] if u['id']==uid)
        mapped=api(worldpath+'/map',token=token);mapped=mapped if isinstance(mapped,list) else mapped['tiles']
        if current['status']!='garrison':saw_away=True
        if saw_away and current['status']=='garrison':break
        time.sleep(.5)
    else:raise AssertionError({'unit_not_home':current})
    assert {t.get('tier') for t in mapped}=={'live','remembered','fog'},'three tiers after scout return'
    playwright=sync_playwright().start();browser=playwright.chromium.launch()
    page=browser.new_page(viewport={'width':1280,'height':900});page.on('pageerror',lambda e:errors.append(str(e)))
    page.goto(base+'/',wait_until='domcontentloaded');page.evaluate('t=>localStorage.setItem("poleia_token",t)',token)
    page.context.add_cookies([{'name':'poleia_token','value':token,'url':base}]);page.goto(base+'/play',wait_until='networkidle')
    page.wait_for_function('window.openDrawer !== undefined')
    for brief in page.locator('.lb-dismiss').all():
        if brief.is_visible():brief.click()
    center=next(t for t in mapped if t.get('tier')=='remembered' and max(abs(t['q']-homeq),abs(t['r']-homer),abs(t['q']+t['r']-homeq-homer))>=3)
    for label,width,height in [('desktop',1280,900),('mobile',390,844)]:
        page.set_viewport_size({'width':width,'height':height})
        page.evaluate("""async t=>{
          const {State}=await import('/static/js/megaron/state.js');
          const {hexPx,SCALE}=await import('/static/js/megaron/render/map.js');
          const rect=document.getElementById('hex-canvas').getBoundingClientRect(),p=hexPx(t.q,t.r);
          State.camera.zoom=0.75;State.camera.x=rect.width/2-p.x*SCALE*State.camera.zoom;State.camera.y=rect.height/2-p.y*SCALE*State.camera.zoom;State.selectedHex=null;State.dirty=true;
        }""",center)
        page.mouse.move(1,1);page.wait_for_timeout(250);page.screenshot(path=str(OUT/(label+'.png')))
    assert not errors,errors
    proof={'health':health,'mode':MODE,'home':[homeq,homer],'target':target,'forecast':forecast,'receipt':receipt,'final_unit':current,'tiles':mapped,'center':center,'browser_errors':errors,'sql_mutations':False}
    (OUT/'proof.json').write_text(json.dumps(proof,indent=2)+'\n');print(json.dumps({'health':health,'mode':MODE,'three_tiers':True,'scout_return_home':True}))
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
