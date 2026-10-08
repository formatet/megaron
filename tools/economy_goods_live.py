#!/usr/bin/env python3
"""N: real multi-good standing order and unchanged internal transfer.
Usage: python3 tools/economy_goods_live.py OUT BUILD_COMMIT baseline|after [WEB_DIR]
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
PREFIX = 'megaron-goods-' + secrets.token_hex(4)
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
    assert chosen,data
    unit=chosen[0]
    def provinces():
        p=api(worldpath+'/provinces',token=token)
        return p if isinstance(p,list) else p['provinces']
    city=next(p for p in provinces() if p.get('settlement_id')==founded['settlement_id'])
    home=(city.get('q',city.get('map_q')),city.get('r',city.get('map_r')))
    def dist(a,b):return max(abs(a[0]-b[0]),abs(a[1]-b[1]),abs(sum(a)-sum(b)))
    colony=None;journeys=[];visited={home};position=home
    for leg in range(12):
        tiles=api(worldpath+'/map',token=token)
        tiles=tiles if isinstance(tiles,list) else tiles['tiles']
        candidates=[t for t in tiles if t.get('terrain') not in ('fog','river','river_ford','deep_sea','coastal_sea','mountain_limestone','mountain_red') and t.get('q') is not None and (t['q'],t['r']) not in visited]
        candidates.sort(key=lambda t:(dist((t['q'],t['r']),home)>=5,dist((t['q'],t['r']),home),-dist((t['q'],t['r']),position)),reverse=True)
        dispatched=False
        for t in candidates:
            target=(t['q'],t['r']);colonize=dist(target,home)>=5
            body={'target_q':target[0],'target_r':target[1],**({'intent':'colonize','name':'Kyme'} if colonize else {})}
            try:
                preview=api(worldpath+'/units/'+unit['id']+'/march-preview?target_q='+str(target[0])+'&target_r='+str(target[1]),token=token)
                if not preview.get('available') and preview.get('reason')!='courier_required':continue
                order=api(worldpath+'/units/'+unit['id']+'/march','POST',body,token)
            except urllib.error.HTTPError:continue
            journeys.append({'body':body,'receipt':order});dispatched=True;visited.add(target);print('leg',leg,body,order,flush=True)
            for _ in range(180):
                time.sleep(1);colony=next((p for p in provinces() if p.get('own') and p.get('name')=='Kyme'),None)
                if colony:break
                moving=next((u for u in api(worldpath+'/units',token=token)['units'] if u['id']==unit['id']),None)
                if moving and moving['status'] in ('positioned','garrison') and (moving.get('q'),moving.get('r'))==target:break
            else:raise RuntimeError('march/colony timeout')
            position=target;break
        if colony:break
        assert dispatched,'no reachable ordinary land site'
    else:raise RuntimeError('could not found colony through ordinary marches')
    from playwright.sync_api import sync_playwright
    playwright=sync_playwright().start();browser=playwright.chromium.launch(ignore_default_args=['--disable-dev-shm-usage'])
    page=browser.new_page(viewport={'width':1280,'height':900});page.on('pageerror',lambda e:errors.append(str(e)))
    page.goto(base+'/',wait_until='domcontentloaded');page.evaluate('t=>localStorage.setItem("poleia_token",t)',token)
    page.context.add_cookies([{'name':'poleia_token','value':token,'url':base}]);page.goto(base+'/play',wait_until='networkidle');page.wait_for_function('window.openDrawer !== undefined')
    posts=[]
    page.on('request',lambda r:posts.append({'url':r.url.split(worldpath)[-1],'body':r.post_data_json}) if r.method=='POST' and (r.url.endswith('/standing-orders') or r.url.endswith('/trade')) else None)
    page.evaluate("window.openDrawer('economy')");page.locator('#economy-body [data-tab=automation]').click();page.wait_for_selector('#ec-so-from')
    page.locator('#ec-so-from').select_option(city['settlement_id']);page.locator('#ec-so-to').select_option(colony['settlement_id']);page.locator('#ec-so-crew').select_option('to')
    if MODE=='baseline':
        page.locator('#ec-so-out').fill('grain:200,fish:50');page.locator('#ec-so-home').fill('silver:0,stone:20')
    else:
        for group,pairs in [('out',[('grain','200'),('fish','50')]),('home',[('silver','0'),('stone','20')])]:
            for i,(good,amount) in enumerate(pairs):
                if i:page.locator('#ec-so-'+group+'-add').click()
                row=page.locator('#ec-so-'+group+' [data-good-row]').nth(i)
                row.locator('select').select_option(good);row.locator('input').fill(amount)
    def pictures(kind):
        for label,w,h in [('desktop',1280,900),('mobile',390,844)]:
            page.set_viewport_size({'width':w,'height':h});page.wait_for_timeout(100)
            page.locator('#drawer-economy').screenshot(path=str(OUT/(kind+'-'+label+'.png')))
            assert page.locator('#economy-body').evaluate('(e)=>e.scrollWidth<=e.clientWidth'),'horizontal overflow'
    pictures('automation')
    with page.expect_response(lambda r:r.request.method=='POST' and r.url.endswith('/standing-orders')) as response:
        page.get_by_role('button',name='Create route',exact=True).click()
    receipt=response.value;assert receipt.status==201,(receipt.status,receipt.text());order=receipt.json()
    expected={'from_settlement_id':city['settlement_id'],'to_settlement_id':colony['settlement_id'],'crewed_by_settlement_id':colony['settlement_id'],'outbound':[{'good_key':'grain','threshold':200},{'good_key':'fish','threshold':50}],'return':[{'good_key':'silver','floor':0},{'good_key':'stone','floor':20}]}
    assert posts[-1]['body']==expected,(posts[-1],expected)
    page.locator('#economy-body [data-tab=transfer]').click();page.wait_for_selector('#ec-tr-good option[value=grain]',state='attached')
    page.locator('#ec-tr-from').select_option(city['id']);page.locator('#ec-tr-to').select_option(colony['settlement_id']);page.locator('#ec-tr-good').select_option('grain');page.locator('#ec-tr-qty').fill('1')
    with page.expect_response(lambda r:r.request.method=='POST' and r.url.endswith('/trade')) as response:
        page.get_by_role('button',name='Transfer →',exact=True).click()
    transfer=response.value;assert transfer.status in (200,201),(transfer.status,transfer.text())
    assert posts[-1]['body']=={'destination_id':colony['settlement_id'],'good_key':'grain','quantity':1}
    pictures('transfer');assert not errors,errors
    proof={'health':health,'mode':MODE,'city':city,'colony':colony,'journeys':journeys,'posts':posts,'standing_receipt':order,'transfer_receipt':transfer.json(),'browser_errors':errors,'sql_mutations':False}
    (OUT/'proof.json').write_text(json.dumps(proof,indent=2)+'\n');print(json.dumps({'health':health,'mode':MODE,'posts':posts}),flush=True)
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
