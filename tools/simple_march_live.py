#!/usr/bin/env python3
"""Real shared march menu via map click, per-unit audit and garrison.
Usage: python3 tools/simple_march_live.py OUT BUILD_COMMIT baseline|map|war|land
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

ROOT = Path(__file__).resolve().parents[1]
OUT = Path(sys.argv[1]).resolve()
EXPECTED = sys.argv[2]
MODE = sys.argv[3] if len(sys.argv) > 3 else "cli"
assert MODE in ("baseline", "map", "war", "land")
OUT.mkdir(parents=True, exist_ok=True)
PREFIX = 'megaron-simple-march-' + secrets.token_hex(4)
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
    api(worldpath+'/join', 'POST', {}, token)
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
    nearby=[t for t in tiles if t['terrain'] not in ('fog','coastal_sea','deep_sea','mountain_limestone','mountain_red') and not any(p.get('q')==t['q'] and p.get('r')==t['r'] for p in provinces) and max(abs(t['q']-homeq),abs(t['r']-homer),abs(t['q']+t['r']-homeq-homer))>=1 and max(abs(t['q']-homeq),abs(t['r']-homer),abs(t['q']+t['r']-homeq-homer))<=2]
    cargo_id=None
    if MODE=='land':
        deadline=time.monotonic()+120
        while True:
            ready=api(worldpath+'/units',token=token)['units']
            ship=next((u for u in ready if u['category']=='naval' and u['status']=='garrison' and u.get('deployable')),None)
            if ship:break
            if time.monotonic()>deadline:raise AssertionError({'no_ready_ship':ready})
            time.sleep(.5)
        cargo_id=chosen[0]['id'];api(worldpath+'/units/'+ship['id']+'/load','POST',{'unit_id':cargo_id},token)
        chosen=[ship,chosen[1]]
        nearby=[t for t in nearby if any(s['terrain'] in ('coastal_sea','deep_sea') and max(abs(t['q']-s['q']),abs(t['r']-s['r']),abs(t['q']+t['r']-s['q']-s['r']))==1 for s in tiles)]
    centre=min(nearby,key=lambda t:(abs(t['r']-homer),abs(t['q']-homeq-4),t['q'],t['r']))
    q,r=centre['q'],centre['r']
    cfg=OUT/'private-config.json';cfg.write_text(json.dumps({'server':base,'token':token,'world_id':world}));cfg.chmod(0o600)
    cli_env={'HOME':os.environ['HOME'],'PATH':os.environ['PATH'],'POLEIA_CONFIG':str(cfg)}
    def cli(*args):return json.loads(subprocess.check_output([str(OUT/'keryx'),*args,'--json'],env=cli_env,text=True))
    if True:
        from playwright.sync_api import sync_playwright, expect
        playwright = sync_playwright().start()
        browser = playwright.chromium.launch()
        page = browser.new_page(viewport={'width': 1280, 'height': 900})
        page.on('pageerror', lambda error: errors.append(str(error)))
        page.goto(base+'/', wait_until='domcontentloaded')
        page.evaluate('t=>localStorage.setItem("poleia_token",t)', token)
        page.context.add_cookies([{'name': 'poleia_token', 'value': token, 'url': base}])
        page.goto(base+'/play', wait_until='networkidle')
        page.wait_for_function('window.openDrawer !== undefined')
    from playwright.sync_api import expect
    page.evaluate("window.openDrawer('war')")
    page.wait_for_selector('#ucard-'+chosen[0]['id'])
    brief=page.locator('#lb-war .lb-dismiss')
    if brief.count():brief.click()
    march_button=page.locator(f"button[onclick=\"unitMarch('{chosen[0]['id']}')\"]")
    if MODE=='baseline':
        march_button.click();expect(page.locator('#wmp-q')).to_be_visible()
        page.locator('#drawer-war').screenshot(path=str(OUT/'war-desktop.png'))
        page.set_viewport_size({'width':390,'height':844})
        page.locator('#war-march-panel').scroll_into_view_if_needed()
        page.locator('#drawer-war').screenshot(path=str(OUT/'war-mobile.png'))
        page.set_viewport_size({'width':1280,'height':900})
        page.evaluate("window.closeDrawer('war')")
    elif MODE in ('war','land'):
        march_button.click();assert page.locator('#wmp-q').count()==0
        expect(page.locator('#mctx-hint')).to_contain_text('Choose a destination')
    else:page.evaluate("window.closeDrawer('war')")
    async_points="""async ([q,r,centre])=>{
      const {State}=await import('/static/js/megaron/state.js');
      const {hexPx,SCALE}=await import('/static/js/megaron/render/map.js');
      const p=hexPx(q,r),rect=document.getElementById('hex-canvas').getBoundingClientRect();
      if(centre){State.camera.x=rect.width/2-p.x*State.camera.zoom*SCALE;
                 State.camera.y=rect.height/2-p.y*State.camera.zoom*SCALE;State.dirty=true;}
      return {x:rect.left+State.camera.x+p.x*State.camera.zoom*SCALE,
              y:rect.top+State.camera.y+p.y*State.camera.zoom*SCALE};
    }"""
    count=1 if MODE in ('war','land') else 2
    for label,width,height in [('desktop',1280,900),('mobile',390,844)]:
        if label=='mobile':
            page.locator('.mctx-close').click()
        page.set_viewport_size({'width':width,'height':height})
        if label=='mobile':
            if MODE in ('war','land'):
                page.evaluate("window.openDrawer('war')")
                page.wait_for_selector('#ucard-'+chosen[0]['id']);march_button.click()
        page.evaluate('window.closeInspect()')
        point=page.evaluate(async_points,[q,r,True])
        page.wait_for_function('p=>document.elementFromPoint(p.x,p.y)?.id==="hex-canvas"',arg=point)
        page.mouse.click(point['x'],point['y'],button='left' if MODE in ('war','land') else 'right')
        page.wait_for_selector('#mg-0')
        if MODE!='land' and not page.locator('#mctx-explore-chk').is_checked():page.locator('#mctx-explore-chk').check()
        expect(page.locator('#mctx-name')).to_contain_text(f'({q},{r})')
        assert page.locator('#mg-0').input_value()==('0' if MODE=='baseline' else str(count))
        if MODE=='baseline':assert page.locator('#mctx-more').count()==0
        else:
            assert not page.locator('#mctx-more').evaluate('(e)=>e.open')
            assert not page.locator('#mctx-ticks').is_visible()
        if MODE in ('war','land'):
            assert page.locator('.mctx-input').count()==1
            assert chosen[0]['display_name'] in page.locator('#mctx-units').inner_text()
            assert chosen[1]['display_name'] not in page.locator('#mctx-units').inner_text()
        page.locator('#march-ctx').screenshot(path=str(OUT/('menu-'+label+'.png')))
        assert page.locator('#march-ctx').evaluate('(e)=>e.scrollWidth<=e.clientWidth'),'overflow'
    if MODE!='land':
        if MODE=='baseline':page.locator('#mg-0').fill('2')
        else:page.locator('#mctx-more summary').click()
        page.locator('#mctx-ticks').fill(str(min(14,data['expedition_rules']['max_ticks'])))
        if MODE!='baseline':page.locator('#mctx-more summary').click()
    receipts=[]
    def record(response):
        if '/units/' in response.url and response.url.endswith('/march') and response.request.method=='POST':
            receipts.append({'http_status':response.status,'unit_id':response.url.split('/')[-2],'request':response.request.post_data_json,'receipt':response.json()})
    page.on('response',record);page.locator('#mctx-send').click()
    page.wait_for_function("document.getElementById('mctx-send').style.display==='none'")
    page.wait_for_timeout(200)
    assert len(receipts)==count and all(x['http_status'] in (200,202) for x in receipts),receipts
    if MODE in ('war','land'):assert receipts[0]['unit_id']==chosen[0]['id'],receipts
    assert all(x['request']['intent']==('land' if MODE=='land' else 'explore') for x in receipts)
    history=[];deadline=time.monotonic()+200
    while time.monotonic()<deadline:
        current=[u for u in api(worldpath+'/units',token=token)['units'] if u['id'] in {x['unit_id'] for x in receipts}]
        history.append([{k:u.get(k) for k in ('id','status','arrival_tick','expedition')} for u in current])
        if len(current)==count and all(u['status']=='garrison' for u in current):break
        time.sleep(.5)
    else:raise AssertionError({'not_home':history[-5:]})
    if MODE=='land':
        cargo=next(u for u in api(worldpath+'/units',token=token)['units'] if u['id']==cargo_id)
        assert cargo['status']=='positioned' and cargo['q']==q and cargo['r']==r,cargo
    sql="SELECT event_type,stream_id,count(*) FROM events WHERE world_id='"+world+"' AND event_type IN ('UnitMarchOrdered','OrderDeliveryFailed') GROUP BY event_type,stream_id ORDER BY event_type;"
    audits=command('docker','exec',containers[0],'psql','-U','postgres','-d','simple-recall-all','-Atc',sql)
    assert all('UnitMarchOrdered|'+x['unit_id']+'|1' in audits for x in receipts),audits
    assert 'OrderDeliveryFailed' not in audits,audits
    assert not errors,errors
    proof={'mode':MODE,'browser_errors':errors,'health':health,'receipts':receipts,'history':history,'audit_rows':audits,'all_garrison':True,'sql_mutations':False}
    (OUT/'proof.json').write_text(json.dumps(proof,indent=2)+'\n')
    print(json.dumps({'health':health,'mode':MODE,'actual_marches':len(receipts),'all_garrison':True}))
except Exception:
    if browser is not None:
        try:
            page.screenshot(path=str(OUT/'failure.png'))
            (OUT/'failure-ui.txt').write_text(page.locator('#march-ctx').evaluate('(e)=>e.outerHTML'))
        except Exception:pass
    raise
finally:
    if browser is not None: browser.close()
    if playwright is not None: playwright.stop()
    if proc is not None:proc.terminate();proc.wait(timeout=20)
    for name in containers:subprocess.run(['docker','rm','-fv',name],capture_output=True)
    if (OUT/'private-config.json').exists():(OUT/'private-config.json').unlink()
