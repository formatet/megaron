#!/usr/bin/env python3
"""Real batch recall via register/join/found, web or CLI, to garrison.
Usage: python3 tools/recall_live.py OUT BUILD_COMMIT web|cli|single [single-delay-seconds]
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
from playwright.sync_api import sync_playwright, expect

ROOT = Path(__file__).resolve().parents[1]
OUT = Path(sys.argv[1]).resolve()
EXPECTED, MODE = sys.argv[2:4]
assert MODE in ('web', 'cli', 'single')
OUT.mkdir(parents=True, exist_ok=True)
PREFIX = 'megaron-recall-' + secrets.token_hex(4)
containers, errors = [], []
proc = None


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
    pg = launch('pg', 'postgres:16-alpine', ['-e', 'POSTGRES_PASSWORD=recall-pw', '-e', 'POSTGRES_DB=recall'])
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
           'DATABASE_URL': f'postgres://postgres:recall-pw@127.0.0.1:{pg}/recall?sslmode=disable',
           'REDIS_URL': f'127.0.0.1:{redis}', 'JWT_SECRET': secrets.token_urlsafe(40),
           'PORT': str(gameport), 'TICK_SECONDS': '6', 'MAP_WIDTH': '56', 'MAP_HEIGHT': '40',
           'WORLD_NAME': 'Recall proof', 'POLEIA_WORLD_START_WANAXES': '1',
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
    token = api('/api/v1/auth/register', 'POST', {'username': 'recall'+secrets.token_hex(4), 'password': secrets.token_urlsafe(32)})['access_token']
    worldpath = '/api/v1/worlds/' + world
    api(worldpath+'/join', 'POST', {}, token)
    founded = api(worldpath+'/founding/settle', 'POST', {'name': 'Nostos'}, token)
    assert founded.get('settlement_id'), founded
    api(worldpath+'/provinces/'+founded['province_id']+'/recruit','POST',{'unit_type':'spearman','count':1},token)
    data = api(worldpath+'/units', token=token)
    candidates = [u for u in data['units'] if u.get('deployable') and u['status']=='garrison' and u['category']=='land']
    assert len(candidates)>=3, data
    chosen = candidates[:3]
    provinces = api(worldpath+'/provinces',token=token)
    provinces = provinces if isinstance(provinces,list) else provinces['provinces']
    city = next(p for p in provinces if p.get('settlement_id')==founded['settlement_id'] or p.get('id')==founded.get('province_id'))
    homeq,homer = city.get('q',city.get('map_q')),city.get('r',city.get('map_r'))
    tilemap=api(worldpath+'/map',token=token)
    tiles=tilemap if isinstance(tilemap,list) else tilemap['tiles']
    nearby=[t for t in tiles if max(abs(t['q']-homeq),abs(t['r']-homer),abs(t['q']+t['r']-homeq-homer))==4]
    centre=min(nearby,key=lambda t:(abs(t['r']-homer),abs(t['q']-homeq-4),t['q'],t['r']))
    q,r=centre['q'],centre['r']
    cfg=OUT/'private-config.json';cfg.write_text(json.dumps({'server':base,'token':token,'world_id':world}));cfg.chmod(0o600)
    cli_env={'HOME':os.environ['HOME'],'PATH':os.environ['PATH'],'POLEIA_CONFIG':str(cfg)}
    def cli(*args): return subprocess.check_output([str(OUT/'keryx'),*args],env=cli_env,text=True)
    for idx,u in enumerate(chosen):
        uq,ur=(q,r) if idx<2 else (homeq,homer+3)
        receipt=json.loads(cli('march','--unit',u['id'],'--target',f'{uq},{ur}','--intent','explore','--ticks',str(data['expedition_rules']['max_ticks']),'--json'))
        assert receipt.get('unit_id')==u['id'] and receipt.get('expedition'),receipt
    single_receipts=[]
    with sync_playwright() as pw:
        browser=pw.chromium.launch();page=browser.new_page(viewport={'width':1280,'height':900})
        page.on('pageerror',lambda e:errors.append(str(e)))
        page.goto(base+'/',wait_until='domcontentloaded')
        page.evaluate('t=>localStorage.setItem("poleia_token",t)',token)
        page.context.add_cookies([{'name':'poleia_token','value':token,'url':base}])
        page.goto(base+'/play',wait_until='networkidle')
        page.wait_for_function('window.unitRecallAll !== undefined')
        page.evaluate("window.openDrawer('war')")
        page.wait_for_selector('#recall-batch-send')
        if MODE=='web':
            with page.expect_response(lambda r:r.url.endswith('/units/recall') and r.request.method=='POST') as response:
                page.locator('#recall-batch-send').click()
            report=response.value.json();assert response.value.status==200,report
            expect(page.locator('#recall-batch-result')).to_contain_text('recall sent by Runner')
            expect(page.locator('#recall-batch-result')).to_contain_text('Group of two units')
            expect(page.locator('#recall-batch-result')).to_contain_text('Single unit')
            expect(page.locator('#recall-batch-result')).to_contain_text('rejected')
        elif MODE=='single':
            time.sleep(float(sys.argv[4]) if len(sys.argv)>4 else 0)
            # Frozen legacy verbs, on the same actual starting cohorts.
            single_receipts.append(json.loads(cli('recall','--unit',chosen[0]['id'],'--json')))
            single_receipts.append(json.loads(cli('redirect','--unit',chosen[1]['id'],'--target',f'{homeq},{homer}','--json')))
            report={'groups':[], 'results':[{'unit_id':u['id'],'name':u.get('display_name',u['id']),'status':'order_dispatched'} for u in chosen]}
        else:
            report=json.loads(cli('recall','--all','--json'))
            text=cli('recall','--all')
            assert 'rejected' in text and 'Runner is already on its way' in text,text
            (OUT/'repeat-cli.txt').write_text(text)
        dispatched={x['unit_id'] for x in report['results'] if x['status']=='order_dispatched'}
        assert {u['id'] for u in chosen}<=dispatched,report
        assert all(x.get('reason') and x.get('name') for x in report['results'] if x['status']=='rejected'),report
        page.locator('#drawer-war').screenshot(path=str(OUT/'recall-desktop.png'))
        page.set_viewport_size({'width':390,'height':844})
        page.locator('#drawer-war').screenshot(path=str(OUT/'recall-mobile.png'))
        assert page.locator('#drawer-war').evaluate('(e)=>e.scrollWidth<=e.clientWidth'), 'drawer overflows on mobile'
        history=[];legacy_readings=[];deadline=time.monotonic()+120
        while time.monotonic()<deadline:
            units=api(worldpath+'/units',token=token)['units']
            current=[u for u in units if u['id'] in dispatched]
            if MODE=='single':
                reading=command('docker','exec',containers[0],'psql','-U','postgres','-d','recall','-Atc',
                    "SELECT u.id,se.due_tick,u.depart_tick,u.arrive_tick,u.q,u.r,u.target_q,u.target_r FROM units u JOIN scheduled_events se ON se.payload->>'unit_id'=u.id::text AND se.event_type='OrderDelivery' WHERE u.world_id='"+world+"' ORDER BY id;")
                if not legacy_readings or legacy_readings[-1]!=reading: legacy_readings.append(reading)
            history.append([{k:u.get(k) for k in ('id','status','current_q','current_r','target_q','target_r','arrival_tick')} for u in current])
            if len(current)==len(dispatched) and all(u['status']=='garrison' for u in current):break
            time.sleep(.5)
        else:raise AssertionError({'not_all_garrison':history[-10:]})
        assert not errors,errors
        browser.close()
    # Read-only DB evidence; setup and commands were all authenticated player paths.
    sql="SELECT event_type,count(*) FROM scheduled_events WHERE world_id='"+world+"' GROUP BY event_type ORDER BY event_type;"
    evidence=command('docker','exec',containers[0],'psql','-U','postgres','-d','recall','-Atc',sql)
    recalled=command('docker','exec',containers[0],'psql','-U','postgres','-d','recall','-Atc',
        "SELECT count(*) FROM events WHERE world_id='"+world+"' AND event_type='MarchRecalled';")
    redirected=command('docker','exec',containers[0],'psql','-U','postgres','-d','recall','-Atc',
        "SELECT count(*) FROM events WHERE world_id='"+world+"' AND event_type='MarchRedirected';")
    if MODE!='single': assert int(recalled)==len(dispatched), 'garrison alone is insufficient: every recipient must actually receive recall'
    proof={'health':health,'mode':MODE,'report':report,'history':history,'db_counts':evidence,'browser_errors':errors,'legacy_receipts':single_receipts,'legacy_unit_readings':legacy_readings,'recall_audits':int(recalled),'redirect_audits':int(redirected)}
    (OUT/'proof.json').write_text(json.dumps(proof,indent=2)+'\n')
    print(json.dumps({'health':health,'mode':MODE,'dispatched':len(dispatched),'all_garrison':True,'recall_audits':int(recalled),'redirect_audits':int(redirected),'groups':len(report['groups']),'legacy_measurement_only':MODE=='single'}))
finally:
    if proc is not None:
        proc.terminate();proc.wait(timeout=20)
    for name in containers:subprocess.run(['docker','rm','-fv',name],capture_output=True)
    if (OUT/'private-config.json').exists():(OUT/'private-config.json').unlink()
