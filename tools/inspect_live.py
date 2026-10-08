#!/usr/bin/env python3
"""Real Host/foreign-city/FOW inspect panels, baseline/after.
Usage: python3 tools/inspect_live.py OUT BUILD_COMMIT baseline|after [WEB_DIR]
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
PREFIX = 'megaron-inspect-' + secrets.token_hex(4)
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
           'PORT': str(gameport), 'TICK_SECONDS': '6', 'MAP_WIDTH': '30', 'MAP_HEIGHT': '30',
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
    host=next(u for u in api(worldpath+'/units',token=token)['units'] if u['type']=='nomadic_host')
    initial_map=api(worldpath+'/map',token=token)
    initial_provinces=api(worldpath+'/provinces',token=token)
    from playwright.sync_api import sync_playwright,expect
    playwright=sync_playwright().start();browser=playwright.chromium.launch(ignore_default_args=['--disable-dev-shm-usage'])
    page=browser.new_page(viewport={'width':1280,'height':900})
    cdp=page.context.new_cdp_session(page);cdp.send('Network.enable');cdp.send('Network.setCacheDisabled',{'cacheDisabled':True})
    page.on('pageerror',lambda e:errors.append(str(e)))
    page.on('console',lambda msg:errors.append(msg.text) if msg.type=='error' else None)
    page.on('requestfailed',lambda r:errors.append(str(r.failure)) if r.failure!='net::ERR_ABORTED' else None)
    page.goto(base+'/',wait_until='domcontentloaded');page.evaluate('t=>localStorage.setItem("poleia_token",t)',token)
    page.context.add_cookies([{'name':'poleia_token','value':token,'url':base}])
    def ready():
        page.wait_for_function("async()=>{const {State}=await import('/static/js/megaron/state.js');return window.openDrawer && State.founderPhase && State.unitsData.some(u=>u.type==='nomadic_host');}")
        page.evaluate('()=>new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)))')
    def hexclick(q,r):
        xy=page.evaluate("""async p=>{
          const {State}=await import('/static/js/megaron/state.js');const {hexPx,SCALE}=await import('/static/js/megaron/render/map.js');
          const at=hexPx(p.q,p.r),canvas=document.getElementById('hex-canvas');
          State.camera.x=canvas.width/2-at.x*SCALE*State.camera.zoom;State.camera.y=canvas.height/2-at.y*SCALE*State.camera.zoom;State.dirty=true;
          await new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)));
          const rect=canvas.getBoundingClientRect();return {x:rect.left+at.x*SCALE*State.camera.zoom+State.camera.x,y:rect.top+at.y*SCALE*State.camera.zoom+State.camera.y};
        }""",{'q':q,'r':r})
        page.mouse.click(xy['x'],xy['y'])
    def shots(kind):
        for label,width,height in [('desktop',1280,900),('mobile',390,844)]:
            page.set_viewport_size({'width':width,'height':height});page.wait_for_timeout(100)
            page.locator('#inspect-panel').screenshot(path=str(OUT/(kind+'-'+label+'.png')))
            page.screenshot(path=str(OUT/(kind+'-context-'+label+'.png')))
            assert page.locator('#inspect-panel').evaluate('e=>e.scrollWidth<=e.clientWidth'),'inspect overflow'
        page.set_viewport_size({'width':1280,'height':900})
    page.goto(base+'/play',wait_until='networkidle');ready();hexclick(host['q'],host['r']);page.wait_for_selector('#ip-settle-btn')
    phase=api(worldpath+'/founding/status',token=token)
    host_text=page.locator('#ip-body-extra').inner_text()
    if MODE=='baseline':assert 'tick left' in host_text and 'real time' in host_text
    else:
        assert 'Food lasts indefinitely' in host_text
        assert 'Escort pay lasts' in host_text and 'game days' in host_text
        assert 'tick left' not in host_text and 'real time' not in host_text
    shots('host')
    page.locator('.inspect-close').click()
    # Ordinary second player gives this player a destination; no State/DB fixture.
    buddy_token=api('/api/v1/auth/register','POST',{'username':'inspect-neighbour-'+secrets.token_hex(4),'password':secrets.token_urlsafe(32)})['access_token']
    api(worldpath+'/join','POST',{},buddy_token)
    neighbour=api(worldpath+'/founding/settle','POST',{'name':'Kyme'},buddy_token)
    buddy_city=next(p for p in api(worldpath+'/provinces',token=buddy_token) if p.get('settlement_id')==neighbour['settlement_id'])
    def distance(a,b):return max(abs(a['q']-b['q']),abs(a['r']-b['r']),abs(a['q']+a['r']-b['q']-b['r']))
    steps=[];visited=set();marker=None
    # Walk only into known land with a real read-only route forecast. Reveal the
    # foreign city through the same live eyes/FOW APIs the web consumes.
    for _ in range(35):
        provinces=api(worldpath+'/provinces',token=token)
        marker=next((p for p in provinces if p['id']==buddy_city['id']),None)
        mapped=api(worldpath+'/map',token=token)
        known=next((t for t in mapped if t['q']==buddy_city['q'] and t['r']==buddy_city['r'] and t['terrain']!='fog'),None)
        if marker and known:break
        current=next(u for u in api(worldpath+'/units',token=token)['units'] if u['id']==host['id'])
        visited.add((current['q'],current['r']))
        candidates=[t for t in mapped if t['terrain'] not in ('fog','river','river_ford','deep_sea','coastal_sea','mountain_limestone','mountain_red') and (t['q'],t['r']) not in visited and (t['q'],t['r'])!=(buddy_city['q'],buddy_city['r'])]
        candidates.sort(key=lambda t:(distance(t,buddy_city),distance(t,current)))
        target=None
        for t in candidates:
            preview=api(worldpath+'/units/'+host['id']+'/march-preview?target_q='+str(t['q'])+'&target_r='+str(t['r']),token=token)
            if preview.get('available'):
                target=t;break
        assert target,'no reachable known land towards neighbour'
        receipt=api(worldpath+'/units/'+host['id']+'/march','POST',{'target_q':target['q'],'target_r':target['r']},token)
        steps.append({'target':target,'preview':preview,'receipt':receipt})
        deadline=time.monotonic()+240
        while time.monotonic()<deadline:
            current=next(u for u in api(worldpath+'/units',token=token)['units'] if u['id']==host['id'])
            if current['status']=='positioned' and current['q']==target['q'] and current['r']==target['r']:break
            time.sleep(.5)
        else:raise AssertionError({'host_not_arrived':current})
    assert marker and known,'neighbour must be revealed by ordinary Host movement'
    army=api(worldpath+'/provinces/'+marker['id']+'/army',token=token)
    assert marker['own']==False and army['Spearman']>0,(marker,army)
    page.reload(wait_until='networkidle');ready();hexclick(marker['q'],marker['r'])
    expect(page.locator('#ip-name')).to_have_text('Kyme')
    if MODE=='baseline':
        expect(page.locator('#ip-army')).to_contain_text('DP')
        assert page.locator('#ip-culture-row').is_visible() and page.locator('#ip-walls-row').is_visible()
    else:
        expect(page.locator('#ip-defence')).to_have_text('strong')
        assert page.locator('#ip-culture-row,#ip-walls-row,#ip-army-row').count()==0
        assert 'DP' not in page.locator('#inspect-panel').inner_text()
    expect(page.locator('#ip-owner')).to_have_text(marker['owner'])
    expect(page.get_by_role('button',name='Send Messenger',exact=True)).to_be_visible()
    expect(page.locator('#ip-march-btn')).to_have_text('March here →')
    shots('foreign-city')
    page.locator('#ip-msg-text').fill('Greetings from a traveller.')
    with page.expect_response(lambda r:'/messengers' in r.url and r.request.method=='POST') as response:
        page.get_by_role('button',name='Send Messenger',exact=True).click()
    assert response.value.status==201,response.value.status
    expect(page.locator('#ip-msg-err')).to_have_text('Messenger sent.')
    page.locator('#ip-march-btn').click();expect(page.locator('#march-ctx')).to_be_visible()
    dest=page.evaluate("async()=>{const {State}=await import('/static/js/megaron/state.js');return State.marchCtxDest;}")
    assert dest['q']==marker['q'] and dest['r']==marker['r'] and dest['known'] and dest['isSettlement'],dest
    page.evaluate('window.closeMarchCtx()')
    # A true fog tile must retain the unknown-only panel, even after visiting a city.
    fog=next(t for t in mapped if t['terrain']=='fog')
    hexclick(fog['q'],fog['r']);expect(page.locator('#ip-name')).to_have_text('Unexplored land')
    assert not page.locator('#ip-owner-row').is_visible()
    assert not page.locator('#ip-defence-row' if MODE=='after' else '#ip-army-row').is_visible()
    assert not page.get_by_role('button',name='Send Messenger',exact=True).count()
    shots('fog')
    assert not errors,errors
    proof={'health':health,'mode':MODE,'host_phase':phase,'host_text':host_text,'initial_provinces':initial_provinces,'neighbour':buddy_city,'revealed_marker':marker,'army':army,'walk':steps,'message_status':response.value.status,'march_dest':dest,'fog':fog,'browser_errors':errors,'sql_mutations':False}
    (OUT/'proof.json').write_text(json.dumps(proof,indent=2)+'\n');print(json.dumps({'health':health,'mode':MODE,'walk_steps':len(steps),'message_status':response.value.status,'browser_errors':errors}))
except Exception:
    print('browser errors:',errors,flush=True)
    if browser is not None:
        try:page.screenshot(path=str(OUT/'failure.png'))
        except Exception:pass
    raise
finally:
    if browser is not None:browser.close()
    if playwright is not None:playwright.stop()
    if proc is not None:proc.terminate();proc.wait(timeout=20)
    for name in containers:subprocess.run(['docker','rm','-fv',name],capture_output=True)
