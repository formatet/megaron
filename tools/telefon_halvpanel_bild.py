#!/usr/bin/env python3
"""BILD for the phone inspect panel as a half-height bottom sheet, on the REAL canvas map in Firefox, against the isolated
acceptance world (tools/acceptance.sh up, http://localhost:8097). Writes docs/reviews/telefon-halvpanel/.
Setup is legitimate play: four players, the first founds its city with keryx and marches a spearman out one hex."""
from pathlib import Path
import json, os, subprocess, sys, time
from playwright.sync_api import sync_playwright

ROOT = Path(__file__).resolve().parents[1]
OUT = ROOT / 'docs/reviews/telefon-halvpanel'; OUT.mkdir(parents=True, exist_ok=True)
ACC = str(ROOT / 'tools/acceptance.sh')
KX = '/tmp/claude-1000/kx-acc'; CFG = '/tmp/claude-1000/kx-acc.json'
BASE = 'http://localhost:8097'
sh = lambda *a, **kw: subprocess.run(list(a), capture_output=True, text=True, **kw)
psql = lambda q: sh(ACC, 'psql', q).stdout.strip()
kx = lambda *a: sh(KX, *a, env={**os.environ, 'POLEIA_CONFIG': CFG})
rows = []
def rec(step, observed, ok, shot=''):
    rows.append((step, str(observed).replace('\n', ' '), bool(ok), shot)); print('PASS' if ok else 'FAIL', step, '|', observed, flush=True)

if not os.path.exists(CFG):  # fresh world: build keryx, register four players, found the first city
    sh('go', 'build', '-o', KX, './cmd/keryx/', cwd=ROOT / 'server')
    sh(ACC, 'player', 'hp1', CFG)
    for n in ('hp2', 'hp3', 'hp4'): sh(ACC, 'player', n)
    time.sleep(2)
    if 'Metropolis' not in kx('dossier').stdout and 'capital' not in kx('dossier').stdout:
        print(kx('founding', 'settle', '--yes').stdout[-200:])
cfg = json.load(open(CFG)); WORLD = cfg['world_id']; TOKEN = cfg['token']
UNIT = psql("select id from units where owner_id='%s' and type='spearman' order by id limit 1" % cfg['player_id'])
print('unit', UNIT, flush=True)

PANELS = '''()=>[...document.querySelectorAll('.drawer,.inspect-panel,.march-ctx,.dispatch-window-overlay,.search-overlay,.codex-panel')]
 .filter(e=>getComputedStyle(e).visibility!=='hidden'&&getComputedStyle(e).display!=='none'&&e.getBoundingClientRect().width>0&&e.getBoundingClientRect().height>0).map(e=>e.id||e.className)'''

def master_routes(page):
    """Serve master's three changed files so the desktop picture is master's."""
    def serve(url, body, ct):
        page.route(url, lambda r: r.fulfill(status=200, content_type=ct, body=body))
    for path, url in (('web/static/js/megaron/ui/mobile_panels.js', '**/ui/mobile_panels.js*'), ('web/static/js/megaron/render/map.js', '**/render/map.js*'), ('web/static/megaron.css', '**/megaron.css*')):
        body = sh('git', '-C', str(ROOT), 'show', '55f7e895:' + path).stdout
        ct = 'text/css' if path.endswith('css') else 'application/javascript'
        serve(url, body, ct)

def session(b, w, h, tag, master=False):
    ctx = b.new_context(viewport=dict(width=w, height=h), device_scale_factor=1, has_touch=(w < 500))
    ctx.add_cookies([dict(name='poleia_token', value=TOKEN, url=BASE)])
    p = ctx.new_page(); errs = []
    p.on('pageerror', lambda e: errs.append(str(e)))
    p.on('console', lambda m: errs.append(m.text) if m.type == 'error' and 'favicon' not in m.text else None)
    if master: master_routes(p)
    p.goto(f'{BASE}/world/{WORLD}/map'); p.wait_for_timeout(3500)
    p.wait_for_function("import('/static/js/megaron/state.js').then(m=>(m.State.unitsData||[]).length>=1)")
    return p, errs

def main():
    with sync_playwright() as pw:
        b = pw.firefox.launch()
        W, H = 390, 844
        p, errs = session(b, W, H, 'mobile')
        ev = p.evaluate
        S = lambda js: ev("import('/static/js/megaron/state.js').then(m=>" + js + ")")
        M = lambda js: ev("import('/static/js/megaron/render/map.js').then(m=>" + js + ")")
        cq, cr = S("m.State.provinceData.filter(x=>x.own).map(x=>[x.q,x.r])[0]")
        # a land hex adjacent to the city for the march-out
        adj = S(f"""m.State.tileData.filter(t=>!['fog','coastal_sea','deep_sea','mountain_limestone','mountain_red','river','lake'].includes(t.terrain)
          &&Math.max(Math.abs(t.q-{cq}),Math.abs(t.r-{cr}),Math.abs(t.q+t.r-{cq}-{cr}))===1&&!m.State.provinceData.some(p=>p.q===t.q&&p.r===t.r)).map(t=>[t.q,t.r])""")
        print('city', (cq, cr), 'adjacent land', adj)
        if psql(f"select status from units where id='{UNIT}'") == 'garrison':
            o = kx('march', '--unit', UNIT, '--to', f'{adj[0][0]},{adj[0][1]}'); print(o.stdout[-300:], o.stderr[-300:])
        # screen helpers
        rect = ev("(()=>{const r=document.getElementById('hex-canvas').getBoundingClientRect();return {l:r.left,t:r.top,w:r.width,h:r.height}})()")
        print('canvas', rect)
        sheet = lambda: ev("(()=>{const e=document.getElementById('inspect-panel');const r=e.getBoundingClientRect();return {vis:getComputedStyle(e).display!=='none'&&getComputedStyle(e).visibility!=='hidden',top:r.top,h:r.height,cls:e.className,bottomNav:document.querySelector('.game-nav').getBoundingClientRect().top,close:(()=>{const c=e.querySelector('.inspect-close').getBoundingClientRect();return [c.width,c.height]})()}})()")
        def hex_at(sx, sy): return M(f"m.hexAtScreen({sx - rect['l']},{sy - rect['t']})")
        def hex_y(q, r): return M(f"m.hexClientPos({q},{r}).y")
        def tap(sx, sy): p.touchscreen.tap(sx, sy); p.wait_for_timeout(500)
        def shot(code, name): p.wait_for_timeout(300); f = f'{code}-{name}.png'; p.screenshot(path=str(OUT / f)); return f
        def inert(): return ev("document.getElementById('hex-canvas').inert")
        def tap_hex(sx, sy):
            hx = hex_at(sx, sy); tap(sx, sy); return hx
        def check_visible(code_step, hx):
            s = sheet(); y = hex_y(hx['q'], hx['r'])
            ok = s['vis'] and 'mobile-sheet' in s['cls'] and y < s['top'] and not inert() and s['h'] <= 0.5 * H + 1 and s['top'] + s['h'] <= s['bottomNav'] + 1
            return ok, s, y
        # (a) upper half
        sx, sy = W // 2, int(rect['t'] + rect['h'] * 0.28)
        hx = tap_hex(sx, sy); ok, s, y = check_visible('a', hx)
        f = shot('a', 'tap-upper-half-sheet'); rec('a tap upper hex: sheet, hex visible, map live', f'hex={hx} hexY={y:.0f} sheetTop={s["top"]:.0f} h={s["h"]:.0f} inert={inert()} close={s["close"]}', ok and s['close'][0] >= 44 and s['close'][1] >= 44, f)
        cam0 = S("({x:m.State.camera.x,y:m.State.camera.y})")
        # (c) second tap with sheet open, hex above sheet
        sx2, sy2 = W // 2 + 90, int(rect['t'] + rect['h'] * 0.18)
        hx2 = tap_hex(sx2, sy2); ok2, s2, y2 = check_visible('c', hx2)
        f = shot('c', 'tap-second-hex-sheet-open'); rec('c second hex replaces content, still one sheet', f'hex={hx2} sel={S("m.State.selectedHex")} sheetTop={s2["top"]:.0f}', ok2 and hx2 != hx, f)
        # (b) lower half: hex under where the sheet would be
        p.keyboard.press('Escape') if False else None
        ev("closeInspect()"); p.wait_for_timeout(300)
        sy3 = int(rect['t'] + rect['h'] * 0.88)
        hx3 = hex_at(W // 2, sy3); camB = S("({x:m.State.camera.x,y:m.State.camera.y})")
        tap(W // 2, sy3); ok3, s3, y3 = check_visible('b', hx3); camA = S("({x:m.State.camera.x,y:m.State.camera.y})")
        f = shot('b', 'tap-lower-half-panned'); rec('b lower hex: map panned, hex above sheet', f'hex={hx3} y_before={sy3 - rect["t"]:.0f} y_after={y3:.0f} sheetTop={s3["top"]:.0f} cam {camB}->{camA}', ok3 and camA['y'] < camB['y'], f)
        # no movement when already visible
        ev("closeInspect()"); p.wait_for_timeout(200)
        camC = S("({x:m.State.camera.x,y:m.State.camera.y})"); hx4 = tap_hex(W // 2 - 60, int(rect['t'] + rect['h'] * 0.3)); camD = S("({x:m.State.camera.x,y:m.State.camera.y})")
        rec('b2 already-visible hex does not move the map', f'cam {camC}->{camD}', camC == camD)
        # drag above the sheet pans the map, selects nothing new
        c0 = S("({x:m.State.camera.x,y:m.State.camera.y})")
        p.mouse.move(W // 2, 220)
        p.evaluate("""()=>{const c=document.getElementById('hex-canvas');const fire=(t,x,y)=>c.dispatchEvent(new PointerEvent(t,{clientX:x,clientY:y,pointerId:7,pointerType:'touch',isPrimary:true,bubbles:true,buttons:1}));fire('pointerdown',195,230);for(let i=1;i<=8;i++)fire('pointermove',195+i*8,230+i*5);fire('pointerup',259,270);}""")
        p.wait_for_timeout(300); c1 = S("({x:m.State.camera.x,y:m.State.camera.y})")
        rec('drag above the sheet pans the map', f'cam {c0}->{c1}', c0 != c1)
        # (d) richest panel: rural / any hex with the longest body; scroll the body
        ev("closeInspect()")
        # (e) hex with a force: wait for the marched unit
        for _ in range(90):
            st = psql(f"select status from units where id='{UNIT}'")
            if st in ('positioned',): break
            time.sleep(4)
        print('unit status', st)
        p.reload(); p.wait_for_timeout(3500)
        rect2 = ev("(()=>{const r=document.getElementById('hex-canvas').getBoundingClientRect();return {l:r.left,t:r.top,w:r.width,h:r.height}})()")
        ux, uy = M(f"(u=>[m.hexClientPos({adj[0][0]},{adj[0][1]}).x,m.hexClientPos({adj[0][0]},{adj[0][1]}).y])()")
        print('force hex screen', ux, uy)
        tap(ux, uy)
        s = sheet(); txt = ev("document.getElementById('inspect-panel').innerText")
        f = shot('e1', 'force-hex-sheet'); rec('e1 force hex opens sheet', f'{s} text={txt[:160]!r}', s['vis'] and 'mobile-sheet' in s['cls'], f)
        # (d) long content: expand both force cards, scroll the sheet body
        for i in range(2): p.locator('#inspect-panel details.hex-force summary').nth(i).tap(); p.wait_for_timeout(200)
        p.wait_for_timeout(300)
        s = sheet(); hy = hex_y(adj[0][0], adj[0][1]); sel = S("m.State.selectedHex")
        bodyinfo = ev("(()=>{const b=document.getElementById('ip-body');b.scrollTop=9999;return {sh:b.scrollHeight,ch:b.clientHeight,st:b.scrollTop,ov:getComputedStyle(b).overflowY}})()")
        f = shot('d', 'long-content-scrolled'); rec('d long content scrolls inside the sheet, header and close stay', f'{bodyinfo} sheet={s} hexY={hy:.0f} sel={sel}', bodyinfo['sh'] > bodyinfo['ch'] and s['h'] <= 0.5 * H + 1, f)
        rec('d2 hex still above the sheet after the cards were expanded', f'hexY={hy:.0f} sheetTop={s["top"]:.0f}', hy < s['top'])
        # (e) March / Redirect button
        btn = p.locator('#inspect-panel button[data-hex-march], #inspect-panel button[data-hex-redirect]').first
        if btn.count():
            btn.scroll_into_view_if_needed(); btn.tap(timeout=5000); p.wait_for_timeout(600)
            vis = ev(PANELS); cls = ev("document.body.className"); sheetvis = sheet()['vis']
            f = shot('e2', 'march-pressed-pick-bar'); rec('e2 March: pick bar, sheet hidden', f'panels={vis} body={cls} bar={p.locator("#hexpick-bar").count()}', not sheetvis and 'hex-picking' in cls, f)
            p.locator('#hexpick-bar button', has_text='Cancel').tap(); p.wait_for_timeout(400)
        else:
            rec('e2 March button', 'no data-hex-march/redirect button in the sheet', False)
        # (f) drawer from the sheet
        ev("closeInspect()"); 
        rural = S("(m.State.ruralData||[]).map(r=>[r.q,r.r])[0]||null")
        print('rural', rural)
        if rural:
            rx, ry = M(f"[m.hexClientPos({rural[0]},{rural[1]}).x,m.hexClientPos({rural[0]},{rural[1]}).y]")
            if not (20 < rx < W - 20 and 110 < ry < H - 330): ev(f"(()=>{{import('/static/js/megaron/render/map.js').then(m=>m)}})()")
            tap(rx, ry)
            f1 = shot('f1', 'rural-sheet-before-drawer'); print('rural sheet', sheet())
            btn = p.locator('#ip-rural-city-btn')
            if btn.count():
                btn.tap(); p.wait_for_timeout(700); vis = ev(PANELS); f = shot('f2', 'drawer-full-screen-sheet-gone')
                rec('f drawer from sheet: drawer full screen, sheet gone', f'panels={vis}', 'drawer-city' in vis and 'inspect-panel' not in vis, f)
        else:
            rec('f drawer', 'no rural hex in this world', False)
        rec('console errors', errs, not errs)
        ctx = p.context; ctx.close()
        # (g) desktop, same hex, master vs worktree
        res = {}
        for label, master in (('master', True), ('worktree', False)):
            pd, e2 = session(b, 1280, 800, 'desktop', master)
            pd.evaluate(f"import('/static/js/megaron/render/map.js').then(m=>{{}})")
            hx = pd.evaluate(f"import('/static/js/megaron/render/map.js').then(m=>m.hexClientPos({adj[0][0]},{adj[0][1]}))")
            pd.mouse.click(hx['x'], hx['y']); pd.wait_for_timeout(700)
            f = f'g-desktop-{label}.png'; pd.screenshot(path=str(OUT / f)); res[label] = f
            pd.context.close()
        from PIL import Image, ImageChops  # compare the panel strip: the map's water shimmers and the top bar has a live clock
        A = Image.open(OUT / res['master']).convert('RGB').crop((1040, 44, 1280, 800)); C = Image.open(OUT / res['worktree']).convert('RGB').crop((1040, 44, 1280, 800))
        diff = ImageChops.difference(A, C).getbbox()
        rec('g desktop 1280 unchanged vs master (panel strip x>=1040; the water animates)', f'{res} diff_bbox={diff}', diff is None, res['worktree'])
        b.close()
    (OUT / 'results.json').write_text(json.dumps(rows, indent=1))
    sys.exit(0 if all(r[2] for r in rows) else 1)
main()
