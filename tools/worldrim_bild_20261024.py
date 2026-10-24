#!/usr/bin/env python3
"""World-Rim step 2 look (ported to master 2026-10-24): own fresh rig, Firefox, before/after.
Usage: python3 tools/worldrim_bild_20261024.py OUTPUT
"Before" serves master's render/map.js through page.route; "after" serves this worktree's.
"""
from pathlib import Path
import os, json, secrets, socket, subprocess, tempfile, time, urllib.request, sys
from playwright.sync_api import sync_playwright
ROOT = Path(__file__).resolve().parents[1]
MASTER_MAP = Path('/home/tk/Projects/megaron/web/static/js/megaron/render/map.js').read_text()
OUT = Path(sys.argv[1]).resolve(); OUT.mkdir(parents=True, exist_ok=True)
containers = []; proc = None
clean = {k: os.environ[k] for k in ('HOME', 'PATH')}; clean['CGO_ENABLED'] = '0'
def run(*a, **kw): return subprocess.check_output(a, text=True, **kw).strip()
def launch(kind, image, args):
    name = 'megaron-worldrim-' + secrets.token_hex(4) + '-' + kind; internal = '5432' if kind == 'pg' else '6379'
    run('docker', 'run', '-d', '--rm', '--name', name, '-p', '127.0.0.1::' + internal, *args, image); containers.append(name)
    return json.loads(run('docker', 'inspect', name))[0]['NetworkSettings']['Ports'][internal + '/tcp'][0]['HostPort']
try:
    with tempfile.TemporaryDirectory(prefix='worldrim-') as td:
        tmp = Path(td); commit = run('git', '-C', str(ROOT), 'rev-parse', '--short=8', 'HEAD')
        run('go', 'build', '-ldflags', '-X main.buildCommit=' + commit, '-o', str(tmp / 'temenos'), './cmd/server', cwd=ROOT / 'server', env=clean)
        pg = launch('pg', 'postgres:16-alpine', ['-e', 'POSTGRES_PASSWORD=rim', '-e', 'POSTGRES_DB=rim']); redis = launch('redis', 'redis:7-alpine', [])
        for _ in range(100):
            if subprocess.run(['docker', 'exec', containers[0], 'pg_isready', '-U', 'postgres', '-q'], capture_output=True).returncode == 0: break
            time.sleep(.25)
        with socket.socket() as s: s.bind(('127.0.0.1', 0)); port = s.getsockname()[1]
        base = f'http://127.0.0.1:{port}'
        env = {**clean, 'DATABASE_URL': f'postgres://postgres:rim@localhost:{pg}/rim?sslmode=disable', 'REDIS_URL': 'localhost:' + redis,
               'JWT_SECRET': secrets.token_urlsafe(40), 'PORT': str(port), 'TICK_SECONDS': '60', 'MAP_WIDTH': '64', 'MAP_HEIGHT': '64',
               'WORLD_NAME': 'World-Rim look', 'POLEIA_WORLD_START_WANAXES': '1', 'STATIC_DIR': str(ROOT / 'web/static'),
               'TEMPLATE_DIR': str(ROOT / 'web/templates'), 'CHRONICLE_DIR': str(tmp / 'chronicles'), 'REPORTS_DIR': str(tmp / 'reports')}
        proc = subprocess.Popen([str(tmp / 'temenos')], cwd=ROOT / 'server', env=env, stdout=(OUT / 'server.log').open('w'), stderr=subprocess.STDOUT)
        def api(path, method='GET', data=None, token=None):
            req = urllib.request.Request(base + path, method=method, data=None if data is None else json.dumps(data).encode(),
                                         headers={'Content-Type': 'application/json', **({'Authorization': 'Bearer ' + token} if token else {})})
            return json.load(urllib.request.urlopen(req, timeout=30))
        for _ in range(120):
            try: api('/healthz'); break
            except Exception:
                if proc.poll() is not None: raise RuntimeError('server exited')
                time.sleep(.5)
        world = api('/api/v1/worlds')[0]['id']; wp = '/api/v1/worlds/' + world
        token = api('/api/v1/auth/register', 'POST', {'username': 'leandros-' + secrets.token_hex(3), 'password': secrets.token_urlsafe(30)})['access_token']
        api(wp + '/join', 'POST', {}, token)
        with sync_playwright() as pw:
            browser = pw.firefox.launch()
            for label, body in (('before', MASTER_MAP), ('after', None)):
                for width, height in ((1280, 900), (390, 844)):
                    ctx = browser.new_context(viewport={'width': width, 'height': height})
                    ctx.add_cookies([{'name': 'poleia_token', 'value': token, 'url': base}])
                    ctx.add_init_script('localStorage.setItem("poleia_token",' + json.dumps(token) + ')')
                    page = ctx.new_page()
                    if body is not None:
                        page.route('**/render/map.js*', lambda r: r.fulfill(status=200, content_type='application/javascript', body=body))
                    page.goto(base + '/world/' + world + '/map', wait_until='networkidle'); page.wait_for_timeout(1500)
                    page.evaluate("async()=>{window.__S=(await import('/static/js/megaron/state.js')).State;window.__M=await import('/static/js/megaron/render/map.js')}")
                    # Zoomed all the way out: the whole world and its rim.
                    for _ in range(12): page.evaluate("window.__M.zoom(0.7)")
                    page.evaluate("window.__S.dirty=true"); page.wait_for_timeout(800)
                    page.screenshot(path=str(OUT / f'{label}-out-{width}.png'))
                    # 1:1 at the world's western edge, middle row.
                    page.evaluate("()=>{const s=window.__S,m=window.__M;m.zoom(1/s.camera.zoom);window.centreOn(0,32)}")
                    page.evaluate("window.__S.dirty=true"); page.wait_for_timeout(800)
                    page.screenshot(path=str(OUT / f'{label}-edge-{width}.png'))
                    ctx.close()
            browser.close()
finally:
    if proc: proc.terminate()
    for c in containers: subprocess.run(['docker', 'rm', '-f', c], capture_output=True)
print('done', OUT)
