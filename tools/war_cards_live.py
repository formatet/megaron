#!/usr/bin/env python3
"""Real War cards, More/stance and march/recall to per-unit audit and home.
Usage: python3 tools/war_cards_live.py OUT BUILD_COMMIT baseline|after|naval|restored|restored-naval [WEB_DIR]
OUT contains a freshly built temenos. baseline uses archived unchanged WEB_DIR.
Every arm creates fresh PG16/Redis and uses ordinary player APIs and web clicks;
SQL is read-only audit, no fixtures. Only own processes/containers are removed.
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
assert MODE in ("baseline", "after", "naval", "restored", "restored-naval")
SIMPLIFIED = MODE in ("after", "naval")
NAVAL = MODE in ("naval", "restored-naval")
OUT.mkdir(parents=True, exist_ok=True)
PREFIX = 'megaron-war-cards-' + secrets.token_hex(4)
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
    if NAVAL:
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
    playwright=sync_playwright().start();browser=playwright.chromium.launch()
    page=browser.new_page(viewport={'width':1280,'height':900})
    page.on('pageerror',lambda error:errors.append(str(error)))
    page.goto(base+'/',wait_until='domcontentloaded')
    page.evaluate('t=>localStorage.setItem("poleia_token",t)',token)
    page.context.add_cookies([{'name':'poleia_token','value':token,'url':base}])
    page.goto(base+'/play',wait_until='networkidle')
    page.wait_for_function('window.openDrawer !== undefined')
    uid=chosen[0]['id'];card=page.locator('#ucard-'+uid)
    def war():
        page.evaluate("window.openDrawer('war')")
        page.wait_for_selector('#ucard-'+uid)
        brief=page.locator('#lb-war .lb-dismiss')
        if brief.count():brief.click()
    def shots(prefix):
        for label,width,height in [('desktop',1280,900),('mobile',390,844)]:
            page.set_viewport_size({'width':width,'height':height})
            card.scroll_into_view_if_needed()
            page.locator('#drawer-war').screenshot(path=str(OUT/(prefix+'-'+label+'.png')))
            assert card.evaluate('(e)=>e.scrollWidth<=e.clientWidth'),'card overflow'
            if MODE.startswith('restored'):assert card.locator('details').count()==0,'restored card must have no More'
            if SIMPLIFIED:
                details=card.locator('details')
                assert details.count()==1 and not details.evaluate('(e)=>e.open')
                details.locator('summary').click()
                page.locator('#drawer-war').screenshot(path=str(OUT/(prefix+'-more-'+label+'.png')))
                assert card.evaluate('(e)=>e.scrollWidth<=e.clientWidth'),'More overflow'
                details.locator('summary').click()
        page.set_viewport_size({'width':1280,'height':900})
    war()
    naval_receipts=[]
    if NAVAL:
        ship=next(u for u in data['units'] if u['category']=='naval' and u['status']=='garrison')
        shipcard=page.locator('#ucard-'+ship['id'])
        for action in ['Load','Unload']:
            if SIMPLIFIED:shipcard.locator('details summary').click()
            expect(shipcard.get_by_role('button',name=action,exact=True)).to_be_visible()
            for label,width,height in [('desktop',1280,900),('mobile',390,844)]:
                page.set_viewport_size({'width':width,'height':height})
                shipcard.scroll_into_view_if_needed()
                page.locator('#drawer-war').screenshot(path=str(OUT/('ship-'+action.lower()+'-more-'+label+'.png')))
                assert shipcard.evaluate('(e)=>e.scrollWidth<=e.clientWidth'),'ship More overflow'
            page.set_viewport_size({'width':1280,'height':900})
            with page.expect_response(lambda r:r.url.endswith('/units/'+ship['id']+'/'+action.lower()) and r.request.method=='POST') as pending:
                shipcard.get_by_role('button',name=action,exact=True).click()
            response=pending.value;assert response.status==200,response.status
            naval_receipts.append({'verb':action,'http_status':response.status,'request':response.request.post_data_json,'receipt':response.json()})
            page.wait_for_timeout(300);war()
        finalship=next(u for u in api(worldpath+'/units',token=token)['units'] if u['id']==ship['id'])
        assert not finalship.get('cargo_unit_id'),finalship
    shots('city')
    if NAVAL:
        final_units=api(worldpath+'/units',token=token)['units']
        cargo_id=naval_receipts[0]['request']['unit_id']
        assert next(u for u in final_units if u['id']==cargo_id)['status']=='garrison',final_units
        assert not errors,errors
        proof={'health':health,'mode':MODE,'naval_receipts':naval_receipts,'final_units':final_units,'browser_errors':errors,'sql_mutations':False}
        (OUT/'proof.json').write_text(json.dumps(proof,indent=2)+'\n')
        print(json.dumps({'health':health,'mode':MODE,'load_unload':True}))
        raise SystemExit(0)
    if MODE!='baseline':
        expect(card).to_contain_text('in the city')
        assert card.locator('#ustance-'+uid).is_visible() != SIMPLIFIED
    stance_receipts=[]
    for stance in ['sentry','fortify','none']:
        if SIMPLIFIED:card.locator('details summary').click()
        card.locator('#ustance-'+uid).select_option(stance)
        with page.expect_response(lambda r:r.url.endswith('/units/'+uid+'/stance') and r.request.method=='POST') as pending:
            card.locator('button[onclick="unitStance(\''+uid+'\')"]').click()
        response=pending.value;assert response.status==200,response.status
        stance_receipts.append({'stance':stance,'http_status':response.status,'receipt':response.json()})
        page.wait_for_timeout(300)
        war()
        if MODE!='baseline' and stance=='fortify':assert card.get_by_role('button',name='March',exact=True).count()==0
    card.get_by_role('button',name='March',exact=True).click()
    centre=min(nearby,key=lambda t:(abs(t['r']-homer),abs(t['q']-homeq-4),t['q'],t['r']))
    q,r=centre['q'],centre['r']
    point=page.evaluate("""async ([q,r])=>{
      const {State}=await import('/static/js/megaron/state.js');
      const {hexPx,SCALE}=await import('/static/js/megaron/render/map.js');
      const p=hexPx(q,r),rect=document.getElementById('hex-canvas').getBoundingClientRect();
      State.camera.x=rect.width/2-p.x*State.camera.zoom*SCALE;
      State.camera.y=rect.height/2-p.y*State.camera.zoom*SCALE;State.dirty=true;
      return {x:rect.left+rect.width/2,y:rect.top+rect.height/2};
    }""",[q,r])
    page.evaluate('window.closeInspect()')
    page.wait_for_function('p=>document.elementFromPoint(p.x,p.y)?.id==="hex-canvas"',arg=point)
    page.mouse.click(point['x'],point['y']);page.wait_for_selector('#mg-0')
    if not page.locator('#mctx-explore-chk').is_checked():page.locator('#mctx-explore-chk').check()
    page.locator('#mctx-more summary').click();page.locator('#mctx-ticks').fill('14')
    with page.expect_response(lambda r:r.url.endswith('/units/'+uid+'/march') and r.request.method=='POST') as pending:
        page.locator('#mctx-send').click()
    march=pending.value;assert march.status==202,(march.status,march.json())
    page.locator('.mctx-close').click();war()
    expect(card.get_by_role('button',name='Recall',exact=True)).to_be_visible()
    if MODE!='baseline':
        expect(card).to_contain_text('on the march')
        assert card.locator('#ustance-'+uid).is_visible() != SIMPLIFIED
        assert card.get_by_role('button',name='Redirect',exact=True).is_visible() != SIMPLIFIED
    shots('marching')
    if MODE!='baseline':
        if SIMPLIFIED:card.locator('details summary').click()
        card.get_by_role('button',name='Redirect',exact=True).click()
        card.get_by_text('or type coordinates',exact=True).click()
        expect(card.locator('#uredir-q-'+uid)).to_be_visible()
        expect(card.locator('#uredir-r-'+uid)).to_be_visible()
        if SIMPLIFIED:card.locator('details summary').click()
    recall_attempts=[]
    for attempt in range(45):
        with page.expect_response(lambda r:r.url.endswith('/units/'+uid+'/recall') and r.request.method=='POST') as pending:
            card.get_by_role('button',name='Recall',exact=True).click()
        recall=pending.value
        recall_attempts.append({'http_status':recall.status,'receipt':recall.json()})
        if recall.status==202:break
        assert recall.status==422 and 'catch' in recall.json().get('error',''),recall_attempts
        # A short outbound exploration leg may finish before a Runner can catch it.
        # Wait for the next real leg; never mutate game state or bypass the gate.
        time.sleep(2);war()
        expect(card.get_by_role('button',name='Recall',exact=True)).to_be_visible()
    else:raise AssertionError({'no_catchable_leg':recall_attempts})
    expect(card.locator('#uorder-'+uid)).to_contain_text('order sent by')
    if MODE!='baseline':expect(card.locator('#uorder-'+uid)).to_contain_text('messenger')
    deadline=time.monotonic()+200
    while time.monotonic()<deadline:
        final=next(u for u in api(worldpath+'/units',token=token)['units'] if u['id']==uid)
        if final['status']=='garrison':break
        time.sleep(.5)
    else:raise AssertionError({'not_home':final})
    audits=command('docker','exec',containers[0],'psql','-U','postgres','-d','simple-recall-all','-Atc',
        "SELECT event_type,count(*) FROM events WHERE stream_id='"+uid+"' AND event_type IN ('UnitMarchOrdered','MarchRecalled','UnitStanceChanged','OrderDeliveryFailed') GROUP BY event_type ORDER BY event_type;")
    assert 'UnitMarchOrdered|1' in audits and 'MarchRecalled|1' in audits,audits
    assert 'OrderDeliveryFailed' not in audits,audits
    assert not errors,errors
    proof={'health':health,'mode':MODE,'browser_errors':errors,'stance_receipts':stance_receipts,'naval_receipts':naval_receipts,
        'march':{'http_status':march.status,'request':march.request.post_data_json,'receipt':march.json()},
        'recall':{'http_status':recall.status,'request':recall.request.post_data_json,'receipt':recall.json()},
        'recall_attempts':recall_attempts,'final_unit':final,'audit_rows':audits,'sql_mutations':False}
    (OUT/'proof.json').write_text(json.dumps(proof,indent=2)+'\n')
    print(json.dumps({'health':health,'mode':MODE,'stance_orders':len(stance_receipts),'march_recall_home':True}))
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
