#!/usr/bin/env python3
"""Fresh real-player gift proof: no SQL fixtures, two parties, web+CLI+raid."""
import json, os, secrets, socket, subprocess, sys, time, urllib.error, urllib.request
from pathlib import Path
ROOT=Path(__file__).resolve().parents[1]
OUT=Path(sys.argv[1]).resolve();EXPECTED=sys.argv[2];OUT.mkdir(parents=True,exist_ok=True)
PREFIX='megaron-gift-'+secrets.token_hex(4);containers=[];errors=[];proc=browser=playwright=None
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
           'PORT': str(gameport), 'TICK_SECONDS': '3', 'MAP_WIDTH': '56', 'MAP_HEIGHT': '40',
           'WORLD_NAME': 'Gift proof', 'POLEIA_WORLD_START_WANAXES': '1',
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
    receipts=[];joins=[]
    def register(label):
        token=api('/api/v1/auth/register','POST',{'username':label+secrets.token_hex(4),'password':secrets.token_urlsafe(32)})['access_token']
        j=api('/api/v1/worlds/'+world+'/join','POST',{},token)
        joins.append({'label':label,'tile':j['tile']})
        return token,j
    def dist(a,b):return max(abs(a['Q']-b['Q']),abs(a['R']-b['R']),abs(a['Q']+a['R']-b['Q']-b['R']))
    worldpath='/api/v1/worlds/'+world
    a,aj=register('giver')
    acity=api(worldpath+'/founding/settle','POST',{'name':'Kyme'},a)
    candidates=[]
    for attempt in range(24):
        try:b,bj=register('receiver')
        except urllib.error.HTTPError as e:
            if e.code==409:break
            raise
        candidates.append((dist(aj['tile'],bj['tile']),b,bj))
        if candidates[-1][0]<=6:break
    assert candidates,'no partner spawn'
    _,b,bj=min(candidates,key=lambda v:v[0])
    bcity=api(worldpath+'/founding/settle','POST',{'name':'Nostos'},b)
    source=acity['province_id'];dest=bcity['settlement_id']
    def destinations():return api(worldpath+'/provinces/'+source+'/trade/destinations',token=a)
    if not any(d['settlement_id']==dest for d in destinations()):
        units=api(worldpath+'/units',token=a)['units']
        explorer=next(u for u in units if u['category']=='land' and u['status']=='garrison' and u.get('deployable'))
        receipt=api(worldpath+'/units/'+explorer['id']+'/march','POST',{'intent':'explore','ticks':30,'target_q':bj['tile']['Q'],'target_r':bj['tile']['R']},a)
        receipts.append({'contact_expedition':receipt})
        deadline=time.monotonic()+120
        while time.monotonic()<deadline and not any(d['settlement_id']==dest for d in destinations()):time.sleep(.4)
    assert any(d['settlement_id']==dest for d in destinations()),'physical contact did not reach partner'
    from playwright.sync_api import sync_playwright,expect
    playwright=sync_playwright().start();engine=sys.argv[3] if len(sys.argv)>3 else 'firefox';browser=getattr(playwright,engine).launch()
    def open_player(token):
        context=browser.new_context(viewport={'width':1280,'height':900},has_touch=True)
        context.add_cookies([{'name':'poleia_token','value':token,'url':base}])
        page=context.new_page();page.on('pageerror',lambda e:errors.append(str(e)))
        page.goto(base+'/',wait_until='domcontentloaded');page.evaluate('t=>localStorage.setItem("poleia_token",t)',token)
        page.goto(base+'/play',wait_until='networkidle');page.wait_for_function('window.openDrawer !== undefined');return page
    page=open_player(a)
    page.evaluate("window.openDrawer('economy')")
    page.locator('#economy-body button[data-tab="transfer"]').click()
    page.wait_for_function('dest=>Array.from(document.querySelector("#ec-tr-to").options).some(o=>o.value===dest)',arg=dest)
    option=page.locator('#ec-tr-to option[value="'+dest+'"]');assert '(gift)' in option.inner_text(),option.inner_text()
    page.locator('#ec-tr-to').select_option(dest)
    for label,width,height in [('desktop',1280,900),('mobile',390,844)]:
        page.set_viewport_size({'width':width,'height':height});page.locator('#drawer-economy').screenshot(path=str(OUT/('transfer-'+label+'.png')))
        assert page.locator('#ectab-transfer').evaluate('e=>e.scrollWidth<=e.clientWidth'),'transfer overflow'
    page.set_viewport_size({'width':1280,'height':900})
    def stocks(token,province):return {g['key']:g for g in api(worldpath+'/provinces/'+province+'/goods',token=token)}
    goods=stocks(a,source)
    good=next(k for k,g in goods.items() if k not in ('silver','grain','fish','cult') and g['amount']>=2 and g.get('rate_per_tick',0)==0)
    chosen=['silver',good];outcomes=[]
    def wait_outcome(tid):
        deadline=time.monotonic()+150
        while time.monotonic()<deadline:
            rows=api(worldpath+'/gifts',token=b)
            found=next((g for g in rows if g['body']['transport_id']==tid),None)
            if found:return found
            time.sleep(.3)
        raise AssertionError('no gift outcome: '+tid)
    for index,key in enumerate(chosen):
        delivered=False
        for attempt in range(6):
            before=stocks(b,bcity['province_id']).get(key,{'amount':0})['amount']
            if index==0:
                page.locator('#ec-tr-good').select_option(key);page.locator('#ec-tr-qty').fill('2')
                with page.expect_response(lambda r:r.url.endswith('/provinces/'+source+'/trade') and r.request.method=='POST') as pending:
                    page.locator('#ectab-transfer button').click()
                response=pending.value;assert response.status==201,(response.status,response.text())
                receipt=response.json();expect(page.locator('#ec-tr-result')).to_contain_text('Gift:')
            else:
                config=OUT/'keryx-config.json';config.write_text(json.dumps({'server':base,'token':a,'world_id':world,'province_id':source}))
                cli_env={'HOME':os.environ['HOME'],'PATH':os.environ['PATH'],'POLEIA_CONFIG':str(config)}
                text=subprocess.check_output([str(OUT/'keryx'),'transfer','--good',key,'--qty','2','--dest','Nostos'],env=cli_env,text=True)
                config.unlink();assert 'Gift dispatched' in text,text
                (OUT/'keryx-transfer.txt').write_text(text)
                history=api(worldpath+'/gifts',token=a);receipt={'transport_id':history[0]['body']['transport_id'],'kind':'gift'}
            assert receipt['kind']=='gift',receipt
            outcome=wait_outcome(receipt['transport_id']);after=stocks(b,bcity['province_id']).get(key,{'amount':0})['amount']
            receipts.append({'good':key,'dispatch':receipt,'outcome':outcome,'recipient_before':before,'recipient_after':after})
            if outcome['kind']=='GiftDelivered':
                assert outcome['body']['credited_quantity']==2 and outcome['body']['lost_quantity']==0,outcome
                assert after>=before+2-0.000001,(before,after)
                delivered=True;outcomes.append(outcome);break
        assert delivered,'no delivery within six ordinary dispatches'
    receiver_page=open_player(b)
    receiver_page.evaluate("window.openDrawer('diplomacy')")
    receiver_page.wait_for_selector('.dip-thread');receiver_page.locator('.dip-thread-header').first.click()
    expect(receiver_page.locator('#dtab-threads')).to_contain_text('Gift delivered:')
    for label,width,height in [('desktop',1280,900),('mobile',390,844)]:
        receiver_page.set_viewport_size({'width':width,'height':height});receiver_page.locator('#drawer-diplomacy').screenshot(path=str(OUT/('thread-'+label+'.png')))
        assert receiver_page.locator('#dtab-threads').evaluate('e=>e.scrollWidth<=e.clientWidth'),'thread overflow'
    # Place a real recipient unit on nearby land with a sentry order. Commands
    # travel by runner as usual; never forge positioned units or change ticks.
    bunits=api(worldpath+'/units',token=b)['units']
    guard=next(u for u in bunits if u['category']=='land' and u['status']=='garrison' and u.get('deployable'))
    tiles=api(worldpath+'/map',token=b);tiles=tiles if isinstance(tiles,list) else tiles['tiles']
    nearby=[t for t in tiles if t['terrain'] not in ('fog','coastal_sea','deep_sea','river','mountain_limestone','mountain_red') and 0<max(abs(t['q']-bj['tile']['Q']),abs(t['r']-bj['tile']['R']),abs(t['q']+t['r']-bj['tile']['Q']-bj['tile']['R']))<=2]
    nearby.sort(key=lambda t:max(abs(t['q']-aj['tile']['Q']),abs(t['r']-aj['tile']['R']),abs(t['q']+t['r']-aj['tile']['Q']-aj['tile']['R'])))
    for tile in nearby:
        try:
            march=api(worldpath+'/units/'+guard['id']+'/march','POST',{'target_q':tile['q'],'target_r':tile['r'],'stance':'sentry'},b);break
        except urllib.error.HTTPError as e:
            if e.code!=422:raise
    else:raise AssertionError('no sentry land destination')
    deadline=time.monotonic()+90
    while time.monotonic()<deadline:
        current=next(u for u in api(worldpath+'/units',token=b)['units'] if u['id']==guard['id'])
        if current['status']=='positioned' and current.get('stance')=='sentry':break
        time.sleep(.3)
    assert current['status']=='positioned' and current.get('stance')=='sentry',current
    raid=api(worldpath+'/provinces/'+source+'/trade','POST',{'destination_id':dest,'good_key':good,'quantity':1},a)
    assert raid['category']=='land','raid arm must be land'
    lost=wait_outcome(raid['transport_id']);assert lost['kind']=='GiftLost' and lost['body']['reason']=='intercepted',lost
    notifications=api(worldpath+'/notifications',token=b)
    rows=notifications if isinstance(notifications,list) else notifications.get('notifications',[])
    assert any(n['kind']=='CaravanSeized' and n.get('body',{}).get('transport_id')==raid['transport_id'] for n in rows),rows
    notices=api(worldpath+'/notifications',token=b)
    # SQL is read-only: durable event identity, mover status and manifest.
    audit=command('docker','exec',containers[0],'psql','-U','postgres','-d','simple-recall-all','-tAc',"SELECT event_type,payload FROM events WHERE world_id='"+world+"' AND event_type IN('GiftDelivered','GiftLost','CaravanRaided') ORDER BY id;")
    (OUT/'audit.txt').write_text(audit+'\n')
    assert not errors,errors
    proof={'health':health,'engine':engine,'joins':joins,'recipient_distance':dist(aj['tile'],bj['tile']),'destinations':destinations(),'receipts':receipts,'raid_dispatch':raid,'raid_outcome':lost,'guard':current,'recipient_notifications':notices,'browser_errors':errors,'sql_mutations':False}
    (OUT/'proof.json').write_text(json.dumps(proof,indent=2)+'\n')
    print(json.dumps({'health':health,'two_goods_delivered':chosen,'real_raid':True}))
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
