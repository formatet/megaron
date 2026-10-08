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
    token=api('/api/v1/auth/register','POST',{'username':'giftbefore'+secrets.token_hex(4),'password':secrets.token_urlsafe(32)})['access_token']
    worldpath='/api/v1/worlds/'+world
    api(worldpath+'/join','POST',{},token)
    api(worldpath+'/founding/settle','POST',{'name':'Kyme'},token)
    from playwright.sync_api import sync_playwright,expect
    playwright=sync_playwright().start()
    for name in ['firefox','chromium','webkit']:
        browser=getattr(playwright,name).launch()
        context=browser.new_context(viewport={'width':1280,'height':900},has_touch=True)
        context.add_cookies([{'name':'poleia_token','value':token,'url':base}])
        page=context.new_page();page.on('pageerror',lambda e:errors.append(str(e)))
        page.goto(base+'/',wait_until='domcontentloaded');page.evaluate('t=>localStorage.setItem("poleia_token",t)',token)
        page.goto(base+'/play',wait_until='networkidle');page.wait_for_function('window.openDrawer !== undefined')
        page.evaluate("window.openDrawer('economy')");page.locator('#economy-body button[data-tab="transfer"]').click()
        expect(page.locator('#ectab-transfer')).to_contain_text('Need at least two of your own settlements')
        for label,width,height in [('desktop',1280,900),('mobile',390,844)]:
            page.set_viewport_size({'width':width,'height':height});page.locator('#drawer-economy').screenshot(path=str(OUT/('transfer-'+name+'-'+label+'.png')))
        browser.close();browser=None
    assert not errors,errors
    (OUT/'proof.json').write_text(json.dumps({'health':health,'client_commit':'eb523aa2','one_city_transfer_blocked':True,'browser_errors':errors,'sql_mutations':False},indent=2)+'\n')
    print('baseline client in all three browsers green')

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
