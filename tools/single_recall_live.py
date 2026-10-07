#!/usr/bin/env python3
"""Real single recall AND redirect from player entry to actual audits and garrison.
Usage: python3 tools/single_recall_live.py OUT BUILD_COMMIT [cli|web]
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
assert MODE in ("cli", "web")
OUT.mkdir(parents=True, exist_ok=True)
PREFIX = 'megaron-single-recall-' + secrets.token_hex(4)
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
    pg = launch('pg', 'postgres:16-alpine', ['-e', 'POSTGRES_PASSWORD=single-recall-pw', '-e', 'POSTGRES_DB=single-recall'])
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
           'DATABASE_URL': f'postgres://postgres:single-recall-pw@127.0.0.1:{pg}/single-recall?sslmode=disable',
           'REDIS_URL': f'127.0.0.1:{redis}', 'JWT_SECRET': secrets.token_urlsafe(40),
           'PORT': str(gameport), 'TICK_SECONDS': '6', 'MAP_WIDTH': '56', 'MAP_HEIGHT': '40',
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
    token = api('/api/v1/auth/register', 'POST', {'username': 'single-recall'+secrets.token_hex(4), 'password': secrets.token_urlsafe(32)})['access_token']
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
    nearby=[t for t in tiles if max(abs(t['q']-homeq),abs(t['r']-homer),abs(t['q']+t['r']-homeq-homer))==4]
    centre=min(nearby,key=lambda t:(abs(t['r']-homer),abs(t['q']-homeq-4),t['q'],t['r']))
    q,r=centre['q'],centre['r']
    cfg=OUT/'private-config.json';cfg.write_text(json.dumps({'server':base,'token':token,'world_id':world}));cfg.chmod(0o600)
    cli_env={'HOME':os.environ['HOME'],'PATH':os.environ['PATH'],'POLEIA_CONFIG':str(cfg)}
    def cli(*args):return json.loads(subprocess.check_output([str(OUT/'keryx'),*args,'--json'],env=cli_env,text=True))
    if MODE == 'web':
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
    march_receipts=[]
    for u in chosen:
        march_receipts.append(cli('march','--unit',u['id'],'--target',f'{q},{r}','--intent','explore','--ticks',str(data['expedition_rules']['max_ticks'])))
    if MODE == 'cli':
        receipts=[cli('recall','--unit',chosen[0]['id']),cli('redirect','--unit',chosen[1]['id'],'--target',f'{homeq},{homer}')]
    else:
        page.evaluate("""async ()=>{
          const war=await import('/static/js/megaron/ui/drawers/war.js');
          document.getElementById('drawer-war').classList.add('open');
          await war.loadWarDrawer();
        }""")
        receipts=[]
        first, second = chosen[0]['id'], chosen[1]['id']
        with page.expect_response(lambda x: '/units/'+first+'/recall' in x.url and x.request.method=='POST') as response:
            page.locator(f'button[onclick="unitRecall(\'{first}\')"]').click()
        assert response.value.status==202, response.value.text()
        receipts.append(response.value.json())
        expect(page.locator('#uorder-'+first)).to_contain_text('sent by Runner')
        page.locator(f'button[onclick="unitRedirectToggle(\'{second}\')"]').click()
        page.locator('#uredir-'+second+' a').click()
        page.locator('#uredir-q-'+second).fill(str(homeq))
        page.locator('#uredir-r-'+second).fill(str(homer))
        with page.expect_response(lambda x: '/units/'+second+'/recall' in x.url and x.request.method=='POST') as response:
            page.locator(f'button[onclick="unitRedirect(\'{second}\')"]').click()
        assert response.value.status==202, response.value.text()
        receipts.append(response.value.json())
        expect(page.locator('#uorder-'+second)).to_contain_text('sent by Runner')
        assert not errors, errors

    assert all(x['status']=='order_dispatched' for x in receipts),receipts
    history=[];deadline=time.monotonic()+120
    while time.monotonic()<deadline:
        units=api(worldpath+'/units',token=token)['units']
        current=[u for u in units if u['id'] in {x['id'] for x in chosen}]
        history.append([{k:u.get(k) for k in ('id','status','current_q','current_r','target_q','target_r','arrival_tick','expedition')} for u in current])
        if len(current)==2 and all(u['status']=='garrison' for u in current):break
        time.sleep(.5)
    else:raise AssertionError({'not_home':history[-5:]})
    # Read-only evidence; every setup/order above uses the real player API/CLI.
    sql="SELECT event_type,stream_id,count(*) FROM events WHERE world_id='"+world+"' AND event_type IN ('MarchRecalled','MarchRedirected','OrderDeliveryFailed') GROUP BY event_type,stream_id ORDER BY event_type;"
    audits=command('docker','exec',containers[0],'psql','-U','postgres','-d','single-recall','-Atc',sql)
    assert 'MarchRecalled|'+chosen[0]['id']+'|1' in audits and 'MarchRedirected|'+chosen[1]['id']+'|1' in audits,audits
    assert 'OrderDeliveryFailed' not in audits,audits
    queue=command('docker','exec',containers[0],'psql','-U','postgres','-d','single-recall','-Atc',"SELECT event_type,due_tick,processed_at,failed_at,payload FROM scheduled_events WHERE world_id='"+world+"' AND event_type IN ('OrderDelivery','UnitArrival') ORDER BY id;")
    assert not errors, errors
    proof={'mode':MODE,'browser_errors':errors,'health':health,'march_receipts':march_receipts,'receipts':receipts,'history':history,'audit_rows':audits,'queue_rows':queue,'all_garrison':True}
    (OUT/'proof.json').write_text(json.dumps(proof,indent=2)+'\n')
    print(json.dumps({'health':health,'actual_recall_and_redirect_audits':True,'all_garrison':True}))
finally:
    if browser is not None: browser.close()
    if playwright is not None: playwright.stop()
    if proc is not None:proc.terminate();proc.wait(timeout=20)
    for name in containers:subprocess.run(['docker','rm','-f',name],capture_output=True)
    if (OUT/'private-config.json').exists():(OUT/'private-config.json').unlink()
