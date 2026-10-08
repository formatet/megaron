#!/usr/bin/env python3
"""Real Host/foreign-city/FOW inspect panels, baseline/after.
Usage: python3 tools/diplomacy_live.py OUT BUILD_COMMIT baseline|after [WEB_DIR]
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
PREFIX = 'megaron-diplomacy-' + secrets.token_hex(4)
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
    host=next(u for u in api(worldpath+'/units',token=token)['units'] if u['type']=='nomadic_host')
    # Choose two ordinary spawns on connected land, using read-only metadata.
    # No client payload, position, world state or DB row is changed by selection.
    def landmass(q,r):
        return command('docker','exec','-e','PGOPTIONS=-c default_transaction_read_only=on',containers[0],
            'psql','-U','postgres','-d','simple-recall-all','-tAc',
            "SELECT landmass_id FROM map_tiles WHERE world_id='"+world+"' AND q="+str(q)+" AND r="+str(r))
    actors=[];groups={};viewer={'token':token,'host':host,'joined':joined,'landmass':landmass(host['q'],host['r'])}
    actors.append({'host':host,'landmass':viewer['landmass']});groups[viewer['landmass']]=viewer
    for actor in range(40):
        candidate_token=api('/api/v1/auth/register','POST',{'username':'inspect-neighbour-'+secrets.token_hex(4),'password':secrets.token_urlsafe(32)})['access_token']
        candidate_join=api(worldpath+'/join','POST',{},candidate_token)
        candidate_host=next(u for u in api(worldpath+'/units',token=candidate_token)['units'] if u['type']=='nomadic_host')
        candidate_landmass=landmass(candidate_host['q'],candidate_host['r'])
        actors.append({'host':candidate_host,'landmass':candidate_landmass})
        if candidate_landmass in groups:
            viewer=groups[candidate_landmass];token=viewer['token'];host=viewer['host'];joined=viewer['joined'];own_landmass=viewer['landmass'];buddy_token=candidate_token
            break
        groups[candidate_landmass]={'token':candidate_token,'host':candidate_host,'joined':candidate_join,'landmass':candidate_landmass}
    else:raise AssertionError('no pair of ordinary spawns on same landmass')
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
    page.goto(base+'/play',wait_until='networkidle');ready()
    # The ordinary neighbour founds; the viewer discovers it by walking.
    neighbour=api(worldpath+'/founding/settle','POST',{'name':'Kyme'},buddy_token)
    buddy_city=next(p for p in api(worldpath+'/provinces',token=buddy_token) if p.get('settlement_id')==neighbour['settlement_id'])
    print('actors',json.dumps({'own_host':host,'own_landmass':own_landmass,'neighbour':buddy_city,'spawn_count':len(actors)}),flush=True)
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
        print('walking',json.dumps(steps[-1]),flush=True)
        deadline=time.monotonic()+240
        while time.monotonic()<deadline:
            current=next(u for u in api(worldpath+'/units',token=token)['units'] if u['id']==host['id'])
            if current['status']=='positioned' and current['q']==target['q'] and current['r']==target['r']:break
            time.sleep(.5)
        else:raise AssertionError({'host_not_arrived':current})
    assert marker and known,'neighbour must be revealed by ordinary Host movement'
    current=next(u for u in api(worldpath+'/units',token=token)['units'] if u['id']==host['id'])
    if distance(current,buddy_city)<3:
        candidates=[t for t in mapped if t['terrain'] not in ('fog','river','river_ford','deep_sea','coastal_sea','mountain_limestone','mountain_red') and 3<=distance(t,buddy_city)<=5]
        candidates.sort(key=lambda t:distance(t,current))
        for target in candidates:
            preview=api(worldpath+'/units/'+host['id']+'/march-preview?target_q='+str(target['q'])+'&target_r='+str(target['r']),token=token)
            if preview.get('available'):break
        else:raise AssertionError('no ordinary founding site near neighbour')
        receipt=api(worldpath+'/units/'+host['id']+'/march','POST',{'target_q':target['q'],'target_r':target['r']},token)
        steps.append({'target':target,'preview':preview,'receipt':receipt})
        deadline=time.monotonic()+240
        while time.monotonic()<deadline:
            current=next(u for u in api(worldpath+'/units',token=token)['units'] if u['id']==host['id'])
            if current['status']=='positioned' and current['q']==target['q'] and current['r']==target['r']:break
            time.sleep(.5)
        else:raise AssertionError('founding march did not arrive')
    own_city=api(worldpath+'/founding/settle','POST',{'name':'Nostos'},token)
    own_id=own_city['settlement_id']
    page.reload(wait_until='networkidle')
    page.wait_for_function("async()=>{const {State}=await import('/static/js/megaron/state.js');return State.MY_SETTLEMENT_ID;}")
    def open_diplomacy(p):
        p.evaluate("window.openDrawer('diplomacy')")
        p.wait_for_selector('#dtab-threads')
        p.wait_for_function("!document.getElementById('dtab-threads').querySelector('.loading')")
    def dip_shots(kind):
        for label,width,height in [('desktop',1280,900),('mobile',390,844)]:
            page.set_viewport_size({'width':width,'height':height});page.wait_for_timeout(100)
            page.locator('#drawer-diplomacy').screenshot(path=str(OUT/(kind+'-'+label+'.png')))
            page.screenshot(path=str(OUT/(kind+'-context-'+label+'.png')))
            assert page.locator('#drawer-diplomacy').evaluate('e=>e.scrollWidth<=e.clientWidth'),'drawer overflow'
        page.set_viewport_size({'width':1280,'height':900})
    open_diplomacy(page)
    tabs=page.locator('#diplomacy-body .dtab').all_text_contents()
    if MODE=='baseline':
        assert tabs==['Correspondence','Compose','Cities','Rulers'],tabs
        page.get_by_role('button',name='Cities',exact=True).click()
        expect(page.locator('#dtab-cities')).to_contain_text('Kyme');dip_shots('known')
        page.get_by_role('button',name='Rulers',exact=True).click()
        expect(page.locator('#dtab-rulers')).to_contain_text(buddy_city['owner']);dip_shots('rulers')
        page.get_by_role('button',name='Compose',exact=True).click()
        page.locator('#dip-dest').select_option(neighbour['settlement_id'])
        page.locator('#dip-msg-text').fill('First letter from Nostos.');dip_shots('first-letter')
        with page.expect_response(lambda r:r.request.method=='POST' and r.url.endswith('/messengers')) as sent_response:
            page.get_by_role('button',name='Dispatch →',exact=True).click()
    else:
        assert tabs==['Correspondence','Known'],tabs
        page.get_by_role('button',name='Known',exact=True).click()
        expect(page.locator('#dtab-known')).to_contain_text('Kyme')
        expect(page.locator('#dtab-known')).to_contain_text(buddy_city['owner']);dip_shots('known')
        page.locator('[data-write="'+neighbour['settlement_id']+'"]').last.click()
        page.locator('.dip-inline-compose textarea').fill('First letter from Nostos.');dip_shots('first-letter')
        with page.expect_response(lambda r:r.request.method=='POST' and r.url.endswith('/messengers')) as sent_response:
            page.get_by_role('button',name='Dispatch →',exact=True).click()
    assert sent_response.value.status==201,(sent_response.value.status,sent_response.value.text())
    letter=sent_response.value.json()
    context=browser.new_context(viewport={'width':1280,'height':900})
    context.add_cookies([{'name':'poleia_token','value':buddy_token,'url':base}])
    recipient=context.new_page();recipient.goto(base+'/');recipient.evaluate('t=>localStorage.setItem("poleia_token",t)',buddy_token)
    recipient.goto(base+'/play',wait_until='networkidle');recipient.wait_for_function('window.openDrawer && window.dipReply')
    recipient.on('pageerror',lambda e:errors.append(str(e)))
    recipient.on('console',lambda m:errors.append(m.text) if m.type=='error' else None)
    def wait_for(get, predicate, reason):
        deadline=time.monotonic()+240
        while time.monotonic()<deadline:
            data=get()
            if predicate(data):return data
            time.sleep(.5)
        raise AssertionError(reason)
    wait_for(lambda:api(worldpath+'/messengers/inbox',token=buddy_token),lambda data:any(m['id']==letter['id'] for m in data),'letter not delivered')
    open_diplomacy(recipient);recipient.locator('.dip-thread-header').filter(has_text='Nostos').click()
    expect(recipient.locator('#dip-msg-'+letter['id'])).to_contain_text('First letter from Nostos.')
    recipient.locator('#dip-reply-'+letter['id']).fill('Kyme welcomes your letter.')
    with recipient.expect_response(lambda r:r.request.method=='POST' and r.url.endswith('/reply')) as replied:
        recipient.locator('#dip-reply-row-'+letter['id']).get_by_role('button',name='Reply',exact=True).click()
    assert replied.value.status==200
    returned=wait_for(lambda:api(worldpath+'/settlements/'+own_id+'/messengers',token=token),lambda data:any(m['id']==letter['id'] and m['status']=='arrived' and m.get('reply_text')=='Kyme welcomes your letter.' for m in data),'reply not returned')
    page.reload(wait_until='networkidle');open_diplomacy(page);page.locator('.dip-thread-header').filter(has_text='Kyme').click()
    expect(page.locator('#dtab-threads')).to_contain_text('Kyme welcomes your letter.');dip_shots('returned-reply')
    offers=[]
    for kind in ['buy','sell']:
        compose=page.locator('.dip-inline-compose');compose.locator('textarea').fill('A '+kind+' offer from Nostos.')
        compose.locator('summary').click();cid=compose.get_attribute('id')
        if kind=='buy':
            page.locator('#'+cid+'-good').select_option('grain');page.locator('#'+cid+'-qty').fill('1');page.locator('#'+cid+'-silver').fill('1')
        else:
            compose.locator('input[value="sell"]').check();page.locator('#'+cid+'-offer-good').select_option('grain');page.locator('#'+cid+'-offer-qty').fill('1');page.locator('#'+cid+'-want-silver').fill('1')
        dip_shots('trade-'+kind)
        with page.expect_response(lambda r:r.request.method=='POST' and r.url.endswith('/messengers')) as offered:
            compose.get_by_role('button',name='Dispatch →',exact=True).click()
        assert offered.value.status==201,(offered.value.status,offered.value.text())
        offers.append({'response':offered.value.json(),'request':offered.value.request.post_data_json})
        page.wait_for_timeout(1600)
        if not page.locator('.dip-inline-compose').is_visible():page.locator('.dip-thread-header').filter(has_text='Kyme').click()
    page.evaluate("window.closeDrawer('diplomacy')")
    marker=next(p for p in api(worldpath+'/provinces',token=token) if p.get('settlement_id')==neighbour['settlement_id'])
    hexclick(marker['q'],marker['r']);page.locator('#ip-msg-text').fill('Another letter, written on the map.')
    with page.expect_response(lambda r:r.request.method=='POST' and r.url.endswith('/messengers')) as inspected:
        page.get_by_role('button',name='Send Messenger',exact=True).click()
    assert inspected.value.status==201
    inbox=wait_for(lambda:api(worldpath+'/messengers/inbox',token=buddy_token),lambda data:all(any(m['id']==o['response']['id'] for m in data) for o in offers),'offers not delivered')
    recipient.reload(wait_until='networkidle');open_diplomacy(recipient);recipient.locator('.dip-thread-header').filter(has_text='Nostos').click()
    expect(recipient.locator('#dtab-threads')).to_contain_text('A buy offer from Nostos.')
    expect(recipient.locator('#dtab-threads')).to_contain_text('A sell offer from Nostos.')
    assert recipient.get_by_role('button',name='Accept ✓',exact=True).count()==2
    assert recipient.get_by_role('button',name='Decline ✗',exact=True).count()==2
    assert not errors,errors
    proof={'health':health,'mode':MODE,'tabs':tabs,'own_city':own_city,'neighbour':buddy_city,'walk':steps,'letter_request':sent_response.value.request.post_data_json,'letter':letter,'reply_receipt':replied.value.json(),'returned_letters':returned,'trade_offers':offers,'recipient_inbox':inbox,'inspect_status':inspected.value.status,'browser_errors':errors,'sql_mutations':False,'readonly_actor_landmass':{'own':own_landmass,'actors':actors}}
    (OUT/'proof.json').write_text(json.dumps(proof,indent=2)+'\n');print(json.dumps({'health':health,'mode':MODE,'letter':201,'reply':200,'offers':[201,201],'inspect':201,'browser_errors':errors}),flush=True)
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
