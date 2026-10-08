#!/usr/bin/env python3
"""Real Host summary/Details and inline founding, baseline/after.
Usage: python3 tools/host_simple_live.py OUT BUILD_COMMIT baseline|after [WEB_DIR]
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
PREFIX = 'megaron-host-simple-' + secrets.token_hex(4)
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
    units=api(worldpath+'/units',token=token)['units']
    host=next(u for u in units if u['type']=='nomadic_host')
    timber_probe=None
    if MODE=='after':
        # Choose a normal joined Host whose actual known forecast has timber.
        # This is read-only selection; no game state or payload is fabricated.
        for attempt in range(20):
            phase=api(worldpath+'/founding/status',token=token)
            timber_probe=api(worldpath+'/colonize-preview?q='+str(host['q'])+'&r='+str(host['r'])
                +'&pop='+str(phase['population'])+'&seed='+str(round(phase['grain']['amount']))+'&starter_farm=1',token=token)
            if timber_probe['goods'].get('timber',0)>0:break
            token=api('/api/v1/auth/register','POST',{'username':'host-timber-'+secrets.token_hex(4),'password':secrets.token_urlsafe(32)})['access_token']
            joined=api(worldpath+'/join','POST',{},token)
            host=next(u for u in api(worldpath+'/units',token=token)['units'] if u['type']=='nomadic_host')
        else:raise AssertionError('no natural joined Host with positive timber forecast')
    from playwright.sync_api import sync_playwright, expect
    playwright=sync_playwright().start();browser=playwright.chromium.launch(ignore_default_args=['--disable-dev-shm-usage'])
    page=browser.new_page(viewport={'width':1280,'height':900})
    # Browser-only cache setting; every request reaches the real server.
    cdp=page.context.new_cdp_session(page)
    cdp.send('Network.enable')
    cdp.send('Network.setCacheDisabled',{'cacheDisabled':True})
    page.on('pageerror',lambda error:errors.append(str(error)))
    page.on('console',lambda msg:errors.append('console: '+msg.text) if msg.type=='error' else None)
    page.on('requestfailed',lambda r:errors.append('request: '+r.url+' '+str(r.failure)) if r.failure!='net::ERR_ABORTED' else None)
    page.goto(base+'/',wait_until='domcontentloaded')
    page.evaluate('t=>localStorage.setItem("poleia_token",t)',token)
    page.context.add_cookies([{'name':'poleia_token','value':token,'url':base}])
    page.goto(base+'/play',wait_until='networkidle')
    page.wait_for_function('window.openDrawer !== undefined')
    dialogs=[];orders=[];order_responses=[]
    def dialog(d):
        dialogs.append(d.message)
        if len(dialogs)==1:d.dismiss()
        else:d.accept()
    page.on('dialog',dialog)
    page.on('request',lambda r: orders.append({'method':r.method,'body':r.post_data}) if r.url.endswith('/founding/settle') else None)
    page.on('response',lambda r: order_responses.append(r.status) if r.url.endswith('/founding/settle') else None)
    page.wait_for_function("""async host=>{
      const {State}=await import('/static/js/megaron/state.js');
      return State.founderPhase && State.unitsData.some(u=>u.type==='nomadic_host'&&u.q===host.q&&u.r===host.r)
        && State.tileData.some(t=>t.q===host.q&&t.r===host.r&&t.terrain!=='fog');
    }""",arg=host)
    page.evaluate('()=>new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)))')
    xy=page.evaluate("""async host=>{
      const {State}=await import('/static/js/megaron/state.js');
      const {hexPx,SCALE}=await import('/static/js/megaron/render/map.js');
      const p=hexPx(host.q,host.r),c=State.camera,rect=document.getElementById('hex-canvas').getBoundingClientRect();
      return {x:rect.left+p.x*SCALE*c.zoom+c.x,y:rect.top+p.y*SCALE*c.zoom+c.y};
    }""",host)
    with page.expect_response(lambda r:'/colonize-preview?' in r.url) as forecast_response:
        page.mouse.click(xy['x'],xy['y'])
    page.wait_for_selector('#ip-settle-btn')
    forecast=forecast_response.value.json()
    expect(page.locator('#ip-found-preview')).to_contain_text('Catchment forecast')
    phase=api(worldpath+'/founding/status',token=token)
    def shots(kind):
        page.mouse.move(0,0)
        for label,width,height in [('desktop',1280,900),('mobile',390,844)]:
            page.set_viewport_size({'width':width,'height':height})
            page.locator('#inspect-panel').screenshot(path=str(OUT/(kind+'-'+label+'.png')))
            page.screenshot(path=str(OUT/(kind+'-context-'+label+'.png')))
            assert page.locator('#inspect-panel').evaluate('e=>e.scrollWidth<=e.clientWidth'),'Host overflow'
            button=page.locator('#ip-settle-btn')
            assert button.is_visible() and button.bounding_box()['y']+button.bounding_box()['height']<=height,'Found button reachable'
        page.set_viewport_size({'width':1280,'height':900})
    if MODE=='after':
        details=page.locator('#ip-host-details')
        assert details.count()==1 and not details.evaluate('e=>e.open')
        expect(page.locator('#ip-found-summary')).to_contain_text('Feeds itself: '+('yes' if forecast['grain']['est_net_per_tick']>=0 else 'no'))
        for good,label in [('timber','Timber'),('stone','Stone')]:
            expect(page.locator('#ip-found-summary')).to_contain_text(label+': '+('yes' if forecast['goods'].get(good,0)>0 else 'no'))
        assert forecast['goods'].get('timber',0)>0,'live scenario must exercise actual positive timber'
        expect(page.locator('#ip-found-summary')).to_contain_text('Timber: yes')
        visible=page.locator('#inspect-panel').inner_text()
        assert 'Catchment forecast' not in visible and 'Escort pay' not in visible and 'Produces' not in visible,visible
        assert not page.locator('#ip-deposits-row').is_visible() and not page.locator('#ip-produces-row').is_visible()
        shots('host-closed')
        details.locator('summary').click()
        expect(page.locator('#ip-found-preview')).to_be_visible()
        expect(details).to_contain_text('Food lasts')
        expect(details).to_contain_text('Escort pay lasts')
        expect(details).to_contain_text('Produces')
        expect(details).to_contain_text('Messengers free')
        shots('host-details')
        details.locator('summary').click()
    else:
        assert page.locator('#ip-host-details').count()==0
        shots('host-closed');shots('host-details')
    page.locator('#ip-settle-btn').click()
    assert not dialogs and not orders
    expect(page.locator('#ip-settle-err')).to_contain_text('The host dissolves — forever.')
    shots('host-confirm')
    page.get_by_role('button',name='Keep travelling',exact=True).click();assert not orders
    assert api(worldpath+'/founding/status',token=token)['active'],'cancel keeps Host'
    page.locator('#ip-settle-btn').click()
    page.get_by_role('button',name='Found the metropolis',exact=True).click()
    page.wait_for_function('window.openDrawer !== undefined && !document.getElementById("ip-settle-btn")')
    assert len(orders)==1 and orders[0]['method']=='POST' and json.loads(orders[0]['body'])=={},orders
    assert order_responses==[201],order_responses
    assert not api(worldpath+'/founding/status',token=token)['active']
    assert not any(u['type']=='nomadic_host' for u in api(worldpath+'/units',token=token)['units'])
    provinces=api(worldpath+'/provinces',token=token)
    city=next(p for p in provinces if p.get('own') and p.get('is_capital'))
    assert not errors,errors
    proof={'health':health,'mode':MODE,'host':host,'phase':phase,'forecast':forecast,'timber_positive_probe':timber_probe,'city':city,'native_dialogs':dialogs,'founding_orders':orders,'founding_responses':order_responses,'browser_errors':errors,'sql_mutations':False}
    (OUT/'proof.json').write_text(json.dumps(proof,indent=2)+'\n')
    print(json.dumps({'health':health,'mode':MODE,'native_dialogs':dialogs,'founding_orders':orders,'founding_responses':order_responses,'browser_errors':errors}))
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
