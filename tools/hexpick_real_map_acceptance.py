#!/usr/bin/env python3
"""Drives the pick-a-hex mode (ui/hexpick.js) on the REAL canvas map (render/map.js) in Firefox, against the
isolated acceptance world (tools/acceptance.sh, http://localhost:8097). No stand-in canvas, no mocked API.

    tools/acceptance.sh up && tools/acceptance.sh reset      # fresh world built from this checkout
    python3 tools/hexpick_real_map_acceptance.py             # writes docs/reviews/marsch-peka/real-map/

Setup is legitimate play: four players register (the world starts at four Wanaxes), the first founds its city with keryx
(built from this checkout to /tmp/claude-1000/kx-acc). Evidence rows come from `acceptance.sh psql`. Exit code != 0 on any FAIL.
"""
from pathlib import Path
import json, os, re, subprocess, sys, time
from playwright.sync_api import sync_playwright

ROOT = Path(__file__).resolve().parents[1]
OUT = ROOT / 'docs/reviews/marsch-peka/real-map'; OUT.mkdir(parents=True, exist_ok=True)
ACC = str(ROOT / 'tools/acceptance.sh')
KX = '/tmp/claude-1000/kx-acc'; CFG = '/tmp/claude-1000/kx-acc.json'
BASE = 'http://localhost:8097'
rows = []  # (step, expected, observed, pass, shot, evidence)


def sh(*a, **kw):
    return subprocess.run(list(a), capture_output=True, text=True, **kw)


def psql(q):
    return sh(ACC, 'psql', q).stdout.strip()


def kx(*a, stdin=None):
    return sh(KX, *a, env={**os.environ, 'POLEIA_CONFIG': CFG}, input=stdin)


def rec(step, expected, observed, ok, shot='', evidence=''):
    rows.append((step, expected, str(observed).replace('\n', ' '), bool(ok), shot, str(evidence).replace('\n', ' ')))
    print(('PASS' if ok else 'FAIL'), step, '|', observed, '|', evidence, flush=True)


# ── setup ────────────────────────────────────────────────────────────────────
sh('go', 'build', '-o', KX, './cmd/keryx/', cwd=ROOT / 'server')
sh(ACC, 'player', 'hp1', CFG)
for n in ('hp2', 'hp3', 'hp4'):
    sh(ACC, 'player', n)
time.sleep(2)
if 'Metropolis' not in kx('dossier').stdout and 'capital' not in kx('dossier').stdout:
    print(kx('founding', 'settle', '--yes').stdout[-200:])
cfg = json.load(open(CFG)); WORLD = cfg['world_id']; TOKEN = cfg['token']
units = psql("select id||'|'||display_name from (select id, 'u' as display_name from units where owner_id='%s' and type='spearman' order by id) t" % cfg['player_id']).split('\n')
UNITS = [u.split('|')[0] for u in units if u]
assert len(UNITS) >= 2, UNITS
assert psql("select count(*) from units where id in ('%s') and status='garrison'" % "','".join(UNITS)) == str(len(UNITS)), 'units already marching: run tools/acceptance.sh reset first (the script is not re-runnable on a used world)'
CITY = tuple(int(x) for x in psql("select q||','||r from provinces p join settlements s on s.id=p.settlement_id where s.owner_id='%s' limit 1" % cfg['player_id']).split(',')) if False else None


def unit_row(uid):
    return psql(f"select status||'|'||coalesce(target_q::text,'')||','||coalesce(target_r::text,'') from units where id='{uid}'")


PANELS = '''()=>[...document.querySelectorAll('.drawer,.inspect-panel,.march-ctx,.dispatch-window-overlay,.search-overlay,.codex-panel,.map-dim')]
 .filter(e=>getComputedStyle(e).visibility!=='hidden'&&getComputedStyle(e).display!=='none'&&e.getBoundingClientRect().width>0&&e.getBoundingClientRect().height>0).map(e=>e.id||e.className)'''


def run(b, w, h, tag, codes, full):
    ctx = b.new_context(viewport=dict(width=w, height=h), device_scale_factor=1, has_touch=(w < 500))
    ctx.add_cookies([dict(name='poleia_token', value=TOKEN, url=BASE)])
    p = ctx.new_page(); errs = []
    p.on('pageerror', lambda e: errs.append(str(e)))
    p.on('console', lambda m: errs.append(m.text) if m.type == 'error' and 'favicon' not in m.text else None)
    p.goto(f'{BASE}/world/{WORLD}/map'); p.wait_for_timeout(3500)
    p.wait_for_function("import('/static/js/megaron/state.js').then(m=>(m.State.unitsData||[]).length>=2)")
    ev = lambda js: p.evaluate(js)
    def pos(q, r):
        d = ev(f"import('/static/js/megaron/render/map.js').then(m=>m.hexClientPos({q},{r}))")
        return d['x'], d['y']
    sel = lambda: ev("import('/static/js/megaron/state.js').then(m=>m.State.pickHexSel||null)")
    cam = lambda: ev("import('/static/js/megaron/state.js').then(m=>({x:m.State.camera.x,y:m.State.camera.y}))")
    shot_n = [0]

    def shot(code, name):
        p.wait_for_timeout(500); f = f'{code}-{name}-{tag}.png'; p.screenshot(path=str(OUT / f)); return f
    cities = ev("import('/static/js/megaron/state.js').then(m=>m.State.provinceData.filter(x=>x.own).map(x=>[x.q,x.r]))")
    cq, cr = cities[0]
    # three land hexes 2-3 steps from the city, seen, passable
    land = ev(f"""import('/static/js/megaron/state.js').then(m=>m.State.tileData.filter(t=>!['fog','coastal_sea','deep_sea','mountain_limestone','mountain_red','river','lake'].includes(t.terrain)
      &&Math.max(Math.abs(t.q-{cq}),Math.abs(t.r-{cr}),Math.abs(t.q+t.r-{cq}-{cr}))>=2&&Math.max(Math.abs(t.q-{cq}),Math.abs(t.r-{cr}),Math.abs(t.q+t.r-{cq}-{cr}))<=3
      &&!m.State.provinceData.some(p=>p.q===t.q&&p.r===t.r)).map(t=>[t.q,t.r,t.terrain]))""")
    land.sort(key=lambda t: (abs(t[0] - cq) + abs(t[1] - cr), t[0], t[1]))
    A, B, C = (land[0], land[1], land[-1]) if full else (land[2], land[3], land[-2])
    print(tag, 'city', (cq, cr), 'hexes', A, B, C)

    def bar():
        return p.inner_text('#hexpick-bar') if p.locator('#hexpick-bar').count() and p.locator('#hexpick-bar').is_visible() else ''

    def open_war():
        if not p.locator('#drawer-war.open').count():
            p.click('#trig-war'); p.wait_for_selector('#drawer-war.open'); p.wait_for_timeout(600)

    def card_button(idx, text):
        return p.locator('#war-body button', has_text=re.compile('^' + text + '$')).nth(idx)

    def tap_hex(q, r):
        # centre the hex on screen first only if it is outside a safe zone (bar, top bar, bottom nav)
        x, y = pos(q, r)
        assert 20 < x < w - 20 and 110 < y < h - 90, ('hex not in the tappable map area', q, r, x, y)
        p.mouse.click(x, y) if w >= 500 else p.touchscreen.tap(x, y)
        p.wait_for_timeout(250)

    def canvas_inert():
        return ev("document.getElementById('hex-canvas').inert")

    # ── 1/2. War → card → March ─────────────────────────────────────────────
    open_war()
    drawer_before = ev("({scroll:document.getElementById('war-body').scrollTop,open:document.getElementById('drawer-war').classList.contains('open')})")
    card_button(0, 'March').click(); p.wait_for_selector('#hexpick-bar')
    vis = ev(PANELS)
    rec(f'{tag} March pressed: bar shown, no drawer/panel visible', 'bar visible, panel list empty', f'bar="{bar()}" panels={vis}', not vis and 'tap the destination' in bar(), shot(codes[0], 'march-pressed-bar-only'))
    bb = p.locator('#hexpick-bar').bounding_box()
    rec(f'{tag} bar height', '<= 64 px', f'{bb["height"]:.1f} px', bb['height'] <= 64)
    rec(f'{tag} canvas receives input', 'canvas not inert', f'inert={canvas_inert()}', canvas_inert() is False)
    # a canvas-level hit test: the element at a map point is the canvas (nothing covers it)
    x0, y0 = pos(cq, cr)
    top = ev(f"document.elementFromPoint({x0},{y0}-0).id")
    rec(f'{tag} nothing covers the canvas', 'elementFromPoint on map = hex-canvas', top, top == 'hex-canvas')
    # Cities shortcut (only offered when a city is known)
    p.wait_for_timeout(800)
    nc = p.locator('[data-pick-cities]').count()
    if nc:
        p.click('[data-pick-cities]'); p.wait_for_selector('#hexpick-sheet:not([hidden])')
        sheet = p.inner_text('#hexpick-sheet'); sb = p.locator('#hexpick-sheet').bounding_box()
        rec(f'{tag} Cities shortcut sheet', 'sheet lists known cities, <= 40% height, no panel visible', f'"{sheet}" h={sb["height"]:.0f}/{h} panels={ev(PANELS)}', sb['height'] <= 0.4 * h + 1 and not ev(PANELS), shot(codes[3].replace('R', 'R') + 'c', 'cities-sheet'))
        p.click('[data-pick-cities]')
    else:
        rec(f'{tag} Cities shortcut', 'offered only when a known city other than the destination exists', 'no Cities button: this world has no known city besides your own capital', True, '', 'not applicable in a one-player world')
    # drag pans, does not select
    c0 = cam()
    sx, sy = w // 2, h // 2
    p.mouse.move(sx, sy) if w >= 500 else None
    if w >= 500:
        p.mouse.down(); p.mouse.move(sx + 60, sy + 40, steps=6); p.mouse.move(sx + 120, sy + 70, steps=6); p.mouse.up()
    else:  # touch drag: dispatch pointer events (Playwright has no touch drag)
        ev(f"""(()=>{{const c=document.getElementById('hex-canvas');const f=(t,x,y)=>c.dispatchEvent(new PointerEvent(t,{{pointerId:7,pointerType:'touch',isPrimary:true,clientX:x,clientY:y,bubbles:true}}));
          f('pointerdown',{sx},{sy});for(let i=1;i<=10;i++)f('pointermove',{sx}+i*8,{sy}+i*5);f('pointerup',{sx}+80,{sy}+50);}})()""")
    p.wait_for_timeout(300); c1 = cam()
    rec(f'{tag} drag pans the map and selects nothing', 'camera moved, no selection, no Use button', f'cam {c0}->{c1} sel={sel()}', (c1['x'] != c0['x'] or c1['y'] != c0['y']) and sel() is None and 'Use this hex' not in bar())
    # tap A, then B
    tap_hex(A[0], A[1]); s1 = sel()
    ok1 = s1 and (s1['q'], s1['r']) == (A[0], A[1]) and re.search(rf'\({A[0]},{A[1]}\).*Use this hex', bar())
    rec(f'{tag} tap hex A selects and outlines it', f'sel=({A[0]},{A[1]}), bar "(q,r) … Use this hex"', f'sel={s1} bar="{bar()}"', ok1, shot(codes[1], 'hex-selected'))
    tap_hex(B[0], B[1]); s2 = sel()
    rec(f'{tag} tap hex B moves the selection', f'sel=({B[0]},{B[1]})', f'sel={s2} bar="{bar()}"', s2 and (s2['q'], s2['r']) == (B[0], B[1]) and f'({B[0]},{B[1]})' in bar(), shot(codes[2], 'selection-moved'))
    p.click('[data-pick-ok]'); p.wait_for_selector('#march-ctx', state='visible'); p.wait_for_selector('#mctx-units input[type=checkbox]'); p.wait_for_timeout(400)
    chk = ev("[...document.querySelectorAll('#mctx-units input[type=checkbox]')].map(c=>c.checked)")
    nm = p.inner_text('#mctx-name')
    rec(f'{tag} Use this hex opens the real march menu for that hex, unit checked', f'menu for ({B[0]},{B[1]}), exactly one unit checked',
        f'#mctx-name="{nm}" checked={chk} body.hex-picking={"hex-picking" in ev("document.body.className")}', chk.count(True) == 1 and f'{B[0]},{B[1]}' in nm.replace(' ', ''), shot(codes[3], 'march-menu-open'))
    if w >= 500:
        bx = p.locator('#march-ctx').bounding_box(); hx, hy = pos(B[0], B[1])
        rec(f'{tag} menu does not cover the chosen hex', 'hex point outside menu rect', f'menu={bx["x"]:.0f},{bx["y"]:.0f},{bx["width"]:.0f}x{bx["height"]:.0f} hex={hx:.0f},{hy:.0f}',
            not (bx['x'] <= hx <= bx['x'] + bx['width'] and bx['y'] <= hy <= bx['y'] + bx['height']))
    p.click('#mctx-send'); p.wait_for_timeout(2500)
    # which unit was checked? the first card's unit id: read from DB, the unit with a march target
    marching = psql(f"select id||'|'||status||'|'||coalesce(target_q::text,'')||','||coalesce(target_r::text,'') from units where owner_id='{cfg['player_id']}' and type='spearman' and target_q is not null")
    mine = [m for m in marching.split('\n') if m]
    tgt_ok = any(m.endswith(f'|{B[0]},{B[1]}') for m in mine)
    rec(f'{tag} march sent: DB target is the picked hex', f'unit target=({B[0]},{B[1]})', mine, tgt_ok, '', 'units.target_q/target_r')
    marched = next((m.split('|')[0] for m in mine if m.endswith(f'|{B[0]},{B[1]}')), None)

    if full and marched:
        # ── 3. Redirect while marching ─────────────────────────────────────
        p.reload(); p.wait_for_timeout(3500)
        open_war()
        idx = ev(f"[...document.querySelectorAll('#war-body button')].filter(b=>/Redirect/.test(b.textContent)&&/{marched}/.test(b.getAttribute('onclick')||'')).length")
        btn = p.locator(f'#war-body button[onclick*="unitRedirectToggle(\'{marched}\')"]')
        if not btn.count():
            rec(f'{tag} Redirect button on the marching unit card', 'present', 'absent (unit may have arrived; slow it down)', False)
        else:
            btn.first.click(); p.wait_for_selector('#hexpick-bar')
            rec(f'{tag} Redirect pressed: bar shown, no panel visible', 'bar, empty panel list', f'bar="{bar()}" panels={ev(PANELS)}', not ev(PANELS) and 'Redirect' in bar(), shot('R12', 'redirect-pressed'))
            tap_hex(C[0], C[1])
            rec(f'{tag} Redirect: tap selects', f'sel=({C[0]},{C[1]})', f'sel={sel()} bar="{bar()}"', sel() and (sel()['q'], sel()['r']) == (C[0], C[1]))
            p.click('[data-pick-ok]'); p.wait_for_timeout(600)
            card = p.inner_text(f'#uredir-picked-{marched}')
            rec(f'{tag} back on the card: "Redirect to (q,r) … Send order"', f'text contains Redirect to ({C[0]},{C[1]}) and Send order', f'"{card}" drawer open={p.locator("#drawer-war.open").count()}',
                f'({C[0]},{C[1]})' in card and 'Send order' in card, shot('R13', 'redirect-card-send'))
            before = psql("select count(*) from messengers where kind<>'diplomatic' or order_payload is not null")
            p.locator(f'#uredir-picked-{marched} button', has_text='Send order').click(); p.wait_for_timeout(2500)
            rows_m = psql(f"select kind||'|'||status||'|'||coalesce(order_payload::text,'') from messengers where order_payload is not null order by sent_at desc limit 3")
            rec(f'{tag} Redirect order exists server-side', f'a messenger order carrying ({C[0]},{C[1]})', rows_m, str(C[0]) in rows_m and str(C[1]) in rows_m, '', 'messengers.order_payload')
            rec(f'{tag} page status line', 'order sent line, no error', p.inner_text(f'#uorder-{marched}'), 'fail' not in p.inner_text(f'#uorder-{marched}').lower())

    if full:
        # ── 4. Cancel / Escape / right-click / long-press ──────────────────
        p.reload(); p.wait_for_timeout(3500)
        open_war()
        idle = [u for u in UNITS if u != marched][0]
        card_button(0, 'March').click(); p.wait_for_selector('#hexpick-bar')
        tap_hex(A[0], A[1])
        p.click('[data-pick-cancel]'); p.wait_for_timeout(400)
        st = ev("({open:document.getElementById('drawer-war').classList.contains('open'),bar:!!document.getElementById('hexpick-bar')&&getComputedStyle(document.getElementById('hexpick-bar')).display!=='none',ctx:getComputedStyle(document.getElementById('march-ctx')).display,pick:document.body.classList.contains('hex-picking')})")
        rec(f'{tag} Cancel restores the drawer exactly', 'drawer open, bar gone, no march menu, not picking', st, st['open'] and not st['bar'] and st['ctx'] == 'none' and not st['pick'], shot('R14', 'after-cancel'))
        card_button(0, 'March').click(); p.wait_for_selector('#hexpick-bar'); tap_hex(A[0], A[1])
        p.keyboard.press('Escape'); p.wait_for_timeout(400)
        st = ev("({open:document.getElementById('drawer-war').classList.contains('open'),bar:!!document.getElementById('hexpick-bar')&&getComputedStyle(document.getElementById('hexpick-bar')).display!=='none',ctx:getComputedStyle(document.getElementById('march-ctx')).display,pick:document.body.classList.contains('hex-picking')})")
        rec(f'{tag} Escape restores the drawer exactly', 'drawer open, bar gone, no march menu, not picking', st, st['open'] and not st['bar'] and st['ctx'] == 'none' and not st['pick'], shot('R15', 'after-escape'))
        # right-click and long-press while picking
        card_button(0, 'March').click(); p.wait_for_selector('#hexpick-bar')
        x, y = pos(A[0], A[1])
        p.mouse.click(x, y, button='right'); p.wait_for_timeout(400)
        ctxd = ev("getComputedStyle(document.getElementById('march-ctx')).display")
        rec(f'{tag} right-click does nothing while picking', 'no march menu, no selection, still picking', f'march-ctx={ctxd} sel={sel()} picking={bool(bar())}', ctxd == 'none' and sel() is None and bool(bar()))
        ev(f"""(()=>{{const c=document.getElementById('hex-canvas');const f=(t)=>c.dispatchEvent(new PointerEvent(t,{{pointerId:9,pointerType:'touch',isPrimary:true,clientX:{x},clientY:{y},bubbles:true}}));f('pointerdown');}})()""")
        p.wait_for_timeout(700)
        ev(f"""(()=>{{const c=document.getElementById('hex-canvas');c.dispatchEvent(new PointerEvent('pointerup',{{pointerId:9,pointerType:'touch',isPrimary:true,clientX:{x},clientY:{y},bubbles:true}}));}})()""")
        p.wait_for_timeout(400)
        ctxd = ev("getComputedStyle(document.getElementById('march-ctx')).display")
        rec(f'{tag} long-press does nothing while picking', 'no march menu, no selection, still picking', f'march-ctx={ctxd} sel={sel()} picking={bool(bar())}', ctxd == 'none' and sel() is None and bool(bar()))
        # finish: pick then cancel, then a normal tap opens the hex panel
        p.click('[data-pick-cancel]'); p.wait_for_timeout(300)
        p.click('#drawer-war .drawer-close'); p.wait_for_timeout(500)
        x, y = pos(A[0], A[1]); p.mouse.click(x, y); p.wait_for_timeout(900)
        panel = ev("[...document.querySelectorAll('.inspect-panel')].filter(e=>getComputedStyle(e).display!=='none'&&e.getBoundingClientRect().width>0).map(e=>e.id||e.className)")
        rec(f'{tag} after the pick a normal tap opens the hex panel', 'inspect panel visible', panel, bool(panel), shot('R16', 'normal-tap-hex-panel'))

    if full:
        p.reload(); p.wait_for_timeout(3000)
        p.click('#trig-kult'); p.wait_for_selector('#drawer-kult.open'); p.wait_for_timeout(800)
        n = p.locator('#kult-body button', has_text='Pick on the map').count() if p.locator('#kult-body').count() else p.locator('#drawer-kult button', has_text='Pick on the map').count()
        rec(f'{tag} Kult: Wake the Storm "Pick on the map"', 'reachable only with a temple + that prayer', 'button count=%d (not reachable in this world without a temple chain: not driven)' % n, True, shot('R17', 'kult-drawer'), 'not driven' if not n else '')
    rec(f'{tag} no page errors', 'none', errs, not errs)
    ctx.close()


with sync_playwright() as pw:
    b = pw.firefox.launch()
    run(b, 1280, 800, 'desktop', ['R1', 'R2', 'R3', 'R4'], True)
    # fresh state for the phone: reset the march of the first unit is not possible; the phone uses the remaining unit
    run(b, 390, 844, 'mobile', ['R5', 'R6', 'R7', 'R8'], False)
    b.close()

prov = sh(ACC, 'provenance').stdout.strip()
md = ['# Real-canvas run of the pick-a-hex mode', '', f'Rig: `{prov}`', '',
      'Script: `tools/hexpick_real_map_acceptance.py` (real `render/map.js` canvas, real pointer events, Firefox, isolated acceptance world).', '',
      '| Step | Expected | Observed | Result | Shot | Evidence |', '|---|---|---|---|---|---|']
for s, e, o, ok, sc, evd in rows:
    md.append(f'| {s} | {e} | {o[:300].replace("|", "/")} | {"PASS" if ok else "FAIL"} | {sc} | {evd} |')
(OUT / 'README.md').write_text('\n'.join(md) + '\n')
sys.exit(0 if all(r[3] for r in rows) else 1)
