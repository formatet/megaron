#!/usr/bin/env python3
"""Real wandering-host letter from map click to Correspondence.
Usage: python3 tools/host_outbox_live.py OUT SERVER_BUILD_COMMIT baseline|fixed
OUT contains freshly built temenos. Own fresh PG16/Redis; real joins, scouting,
map-click letter and drawer. No SQL fixtures; only own resources are removed.
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
MODE = sys.argv[3] if len(sys.argv) > 3 else "fixed"
assert MODE in ("baseline", "fixed")
OUT.mkdir(parents=True, exist_ok=True)
PREFIX = 'megaron-host-outbox-' + secrets.token_hex(4)
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
    pg = launch('pg', 'postgres:16-alpine', ['-e', 'POSTGRES_PASSWORD=host-outbox-pw', '-e', 'POSTGRES_DB=host-outbox'])
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
           'DATABASE_URL': f'postgres://postgres:host-outbox-pw@127.0.0.1:{pg}/host-outbox?sslmode=disable',
           'REDIS_URL': f'127.0.0.1:{redis}', 'JWT_SECRET': secrets.token_urlsafe(40),
           'PORT': str(gameport), 'TICK_SECONDS': '2', 'MAP_WIDTH': '40', 'MAP_HEIGHT': '30',
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
    assert health['commit'] == EXPECTED and health['migration'] == 160, health
    worlds = api('/api/v1/worlds')
    world = (worlds if isinstance(worlds, list) else worlds['worlds'])[0]['id']
    worldpath = '/api/v1/worlds/' + world
    players=[]
    pair=None
    # Real joins: choose two host spawns close enough for first contact.
    # No fixtures, teleports, SQL writes or fog bypass.
    for i in range(24):
        token=api('/api/v1/auth/register','POST',{'username':'outbox'+secrets.token_hex(4),'password':secrets.token_urlsafe(32)})['access_token']
        api(worldpath+'/join','POST',{},token)
        host=api(worldpath+'/founding/status',token=token)
        for other in players:
            d=max(abs(host['q']-other['host']['q']),abs(host['r']-other['host']['r']),abs(host['q']+host['r']-other['host']['q']-other['host']['r']))
            if 4 < d <= 6: pair=(other,{'token':token,'host':host});break
        players.append({'token':token,'host':host})
        if pair:break
    assert pair, 'no nearby hosts in actual join spawns'
    recipient,sender=pair
    founded=api(worldpath+'/founding/settle','POST',{'name':'First contact'},recipient['token'])
    token=sender['token']
    # First contact requires actual scouting, not knowing a fixture coordinate.
    host=sender['host'];dest=recipient['host']
    distance=lambda q,r:max(abs(q-dest['q']),abs(r-dest['r']),abs(q+r-dest['q']-dest['r']))
    while distance(host['q'],host['r'])>2:
        tiles=api(worldpath+'/map',token=token)
        tiles=tiles if isinstance(tiles,list) else tiles['tiles']
        candidates=[t for t in tiles if t.get('terrain') not in (None,'fog','coastal_sea','deep_sea','river','mountain_limestone','mountain_red') and 0<distance(t['q'],t['r'])<distance(host['q'],host['r'])]
        assert candidates,'no explored land towards first contact'
        target=min(candidates,key=lambda t:distance(t['q'],t['r']))
        api(worldpath+'/units/'+host['host_unit_id']+'/march','POST',{'target_q':target['q'],'target_r':target['r']},token)
        deadline=time.monotonic()+120
        while time.monotonic()<deadline:
            unit=next(u for u in api(worldpath+'/units',token=token)['units'] if u['id']==host['host_unit_id'])
            if unit['status']=='positioned':break
            time.sleep(.25)
        else:raise AssertionError('host did not reach scouting tile')
        host=api(worldpath+'/founding/status',token=token)
    from playwright.sync_api import sync_playwright, expect
    playwright=sync_playwright().start();browser=playwright.chromium.launch()
    page=browser.new_page(viewport={'width':1280,'height':900})
    page.on('pageerror',lambda error: errors.append(str(error)))
    page.goto(base+'/',wait_until='domcontentloaded')
    page.evaluate('t=>localStorage.setItem("poleia_token",t)',token)
    page.context.add_cookies([{'name':'poleia_token','value':token,'url':base}])
    page.goto(base+'/play',wait_until='networkidle')
    page.wait_for_function('window.openDrawer !== undefined')
    # Pan the actual map camera and click the actual city tile.
    target=recipient['host']
    point=page.evaluate("""async ({q,r})=>{
      const {State}=await import('/static/js/megaron/state.js');
      if(State.MY_SETTLEMENT_ID) throw Error('sender already founded');
      const canvas=document.getElementById('hex-canvas');
      const {hexPx,SCALE}=await import('/static/js/megaron/render/map.js');
      const {x,y}=hexPx(q,r);
      State.camera.x=canvas.width/2-x*SCALE;State.camera.y=canvas.height/2-y*SCALE;State.camera.zoom=1;State.dirty=true;
      const box=canvas.getBoundingClientRect();
      return {x:box.left+canvas.width/2,y:box.top+canvas.height/2};
    }""",target)
    page.mouse.click(point['x'],point['y'])
    page.screenshot(path=str(OUT/'map-click.png'))
    (OUT/'map-click.json').write_text(json.dumps(page.evaluate("async()=>{const {State}=await import('/static/js/megaron/state.js');return {camera:State.camera,selected:State.selectedHex,provinces:State.provinceData,founder:State.founderPhase};}"),indent=2))
    page.locator('#ip-msg-text').wait_for(timeout=5000)
    page.locator('#ip-msg-text').fill('Hello from my wandering host')
    with page.expect_response(lambda r:'/founding/messengers' in r.url and r.request.method=='POST') as response:
        page.locator('button.msg-send').click()
    assert response.value.ok,response.value.text()
    receipt=response.value.json()
    expect(page.locator('#ip-msg-err')).to_have_text('Messenger sent.')
    outbox=api(worldpath+'/founding/messengers',token=token)
    assert any(m['message_text']=='Hello from my wandering host' for m in outbox),outbox
    awaitable="""async ()=>{window.openDrawer('diplomacy');const d=await import('/static/js/megaron/ui/drawers/diplomacy.js');await d.loadDiplomacyDrawer();}"""
    page.evaluate(awaitable)
    if MODE=='baseline':expect(page.locator('#dtab-threads')).to_contain_text('No correspondence yet.')
    else:
        expect(page.locator('#dtab-threads')).to_contain_text('First contact')
        expect(page.locator('#dtab-threads')).to_contain_text('Hello from my wandering host')
        page.locator('.dip-thread-header').first.click()
        expect(page.locator('.dip-bubble-out').first).to_contain_text('Hello from my wandering host')
    page.screenshot(path=str(OUT/'host-outbox-desktop.png'))
    page.set_viewport_size({'width':390,'height':844})
    page.screenshot(path=str(OUT/'host-outbox-mobile.png'))
    assert not errors,errors
    proof={'mode':MODE,'health':health,'players_joined':len(players),'sender_host':sender['host'],'recipient_host':recipient['host'],'receipt':receipt,'outbox':outbox,'threads_text':page.locator('#dtab-threads').inner_text(),'browser_errors':errors,'sql_mutations':0}
    (OUT/'proof.json').write_text(json.dumps(proof,indent=2)+'\n')
    print(json.dumps({'mode':MODE,'health':health,'host_message_sent':True,'web_outbox_visible':MODE=='fixed'}))
finally:
    if browser is not None:browser.close()
    if playwright is not None:playwright.stop()
    if proc is not None:proc.terminate();proc.wait(timeout=20)
    for name in containers:subprocess.run(['docker','rm','-fv',name],capture_output=True)
