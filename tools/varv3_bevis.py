#!/usr/bin/env python3
"""First hour, round three: every item proven on the REAL page in Firefox (1280x800 and 390x844), by real
clicks, measuring what is in the viewport after the action. Run it BEFORE the fix (must fail for the item) and AFTER.

Setup (isolated acceptance world only, http://localhost:8097):
  bash tools/acceptance.sh up
  bash tools/acceptance.sh player Varv1 /tmp/claude-1000/v3/Varv1.json   (Varv2..Varv6 likewise)
  python3 tools/varv3_bevis.py setup      # founds Varv1..Varv4 and reveals the map for Varv1 via psql
  python3 tools/varv3_bevis.py 1 3 6      # run chosen items (no argument: all); exit 1 on any failure
Pictures go to docs/reviews/forsta-timmen-varv3/<phase>/ (phase = $PHASE, default 'after')."""
import json, os, subprocess, sys, urllib.request
from pathlib import Path
from playwright.sync_api import sync_playwright

ROOT = Path(__file__).resolve().parents[1]
CFG = Path(os.environ.get('V3_DIR', '/tmp/claude-1000/v3'))
B = 'http://localhost:8097'
PHASE = os.environ.get('PHASE', 'after')
OUT = ROOT / 'docs/reviews/forsta-timmen-varv3' / PHASE
OUT.mkdir(parents=True, exist_ok=True)
SIZES = [(1280, 800), (390, 844)]
results = []


def cfg(name): return json.load(open(CFG / f'{name}.json'))


def api(c, path, method='GET', data=None):
    r = urllib.request.Request(B + path, method=method, data=None if data is None else json.dumps(data).encode(),
                               headers={'Content-Type': 'application/json', 'Authorization': 'Bearer ' + c['token']})
    try:
        with urllib.request.urlopen(r, timeout=30) as x: return x.status, json.load(x)
    except urllib.error.HTTPError as e:
        return e.code, e.read().decode()[:300]


def psql(sql):
    return subprocess.run(['bash', str(ROOT / 'tools/acceptance.sh'), 'psql', sql], capture_output=True, text=True, cwd=ROOT).stdout


def reset_units(name):
    """Put the player's units back in the garrison (acceptance world only: earlier marches used them up)."""
    pid = psql(f"select id from players where username='{name}'").strip()
    psql(f"delete from scheduled_events where event_type like 'UnitArrival%' and processed_at is null and payload::text like any (select '%'||id::text||'%' from units where owner_id='{pid}')")
    psql(f"update units set status='garrison', settlement_id=support_settlement_id, q=null, r=null, target_q=null, target_r=null, depart_tick=null, arrive_tick=null, departs_at=null, arrives_at=null, march_route=null, march_intent=null, stance=null where owner_id='{pid}' and status in ('marching','positioned')")


def chk(item, name, ok, detail=''):
    results.append((item, name, bool(ok)))
    print(f"  [{'PASS' if ok else 'FAIL'}] item {item} · {name}" + (f' — {detail}' if detail else ''), flush=True)


def newpage(pw, name, w, h, wait=4500):
    b = pw.firefox.launch()
    c = cfg(name)
    ctx = b.new_context(viewport=dict(width=w, height=h), device_scale_factor=1, has_touch=w < 500)
    ctx.add_cookies([dict(name='poleia_token', value=c['token'], url=B)])
    p = ctx.new_page()
    p.on('pageerror', lambda e: print('   pageerror', e))
    p.goto(f"{B}/world/{c['world_id']}/map"); p.wait_for_timeout(wait)
    return b, p, c


def shot(p, item, name):
    p.wait_for_timeout(300)
    p.screenshot(path=str(OUT / f'{item:02d}-{name}.png'))


# In the viewport AND inside the visible part of every scrolling ancestor.
IN_VIEW = """sel => { const el = document.querySelector(sel); if (!el) return {exists:false};
  const r = el.getBoundingClientRect(); if (!r.width && !r.height) return {exists:true, visible:false, why:'zero size'};
  let top = 0, left = 0, bottom = innerHeight, right = innerWidth; const why = [];
  for (let a = el.parentElement; a; a = a.parentElement) { const cs = getComputedStyle(a);
    if (/(auto|scroll|hidden|clip)/.test(cs.overflowY + cs.overflowX)) { const q = a.getBoundingClientRect();
      top = Math.max(top, q.top); bottom = Math.min(bottom, q.bottom); left = Math.max(left, q.left); right = Math.min(right, q.right); } }
  const ok = r.top >= top - 1 && r.bottom <= bottom + 1 && r.left >= left - 1 && r.right <= right + 1;
  return {exists:true, visible: ok, rect:[Math.round(r.left), Math.round(r.top), Math.round(r.right), Math.round(r.bottom)], clip:[Math.round(left), Math.round(top), Math.round(right), Math.round(bottom)], text:(el.innerText||'').slice(0,120)}; }"""


def in_view(p, sel): return p.evaluate(IN_VIEW, sel)


def centre_on(p, q, r, fy=0.4):
    return p.evaluate("""a => Promise.all([import('/static/js/megaron/render/map.js'), import('/static/js/megaron/state.js')]).then(([m, s]) => {
      let c = m.hexClientPos(a[0], a[1]); const cv = m.canvas.getBoundingClientRect();
      s.State.camera.x += cv.left + cv.width / 2 - c.x; s.State.camera.y += cv.top + cv.height * a[2] - c.y; s.State.dirty = true;
      return new Promise(res => setTimeout(() => { c = m.hexClientPos(a[0], a[1]); res([c.x, c.y]); }, 500)); })""", [q, r, fy])


def tap(p, x, y, w):
    if w < 500: p.touchscreen.tap(x, y)
    else: p.mouse.click(x, y)


def nav(p, label, w):
    if w < 500: p.evaluate("t => [...document.querySelectorAll('.game-nav .win-trigger, .game-nav button')].find(b => b.textContent.trim().toLowerCase().includes(t))?.click()", label.lower())
    else: p.click(f'.game-nav >> text={label}')
    p.wait_for_timeout(2200)


# ── setup ────────────────────────────────────────────────────────────────────
def setup():
    for n in ['Varv1', 'Varv2', 'Varv3', 'Varv4']:
        c = cfg(n)
        s, d = api(c, f"/api/v1/worlds/{c['world_id']}/founding/settle", 'POST', {'name': 'City' + n if n != 'Varv1' else 'Itanos'})
        print(n, s, str(d)[:60])
    pid = psql("select id from players where username='Varv1'").strip()
    print(psql(f"insert into player_scouted_tiles(world_id,player_id,q,r) select world_id,'{pid}',q,r from map_tiles on conflict do nothing"))


def war_march_to(p, w, q, r, fy=0.7):
    """The player's way: War → the unit's March → tap the hex on the map → Use this hex."""
    nav(p, 'War', w)
    p.locator('#drawer-war').get_by_role('button', name='March', exact=True).first.click(); p.wait_for_timeout(800)
    x, y = centre_on(p, q, r, fy)
    tap(p, x, y, w); p.wait_for_timeout(600)
    p.click('[data-pick-ok]'); p.wait_for_timeout(2500)


def reachable_land(c):
    """A known land hex three steps from the city that a plain march accepts (found through the API)."""
    s, units = api(c, f"/api/v1/worlds/{c['world_id']}/units"); uid = units['units'][0]['id']
    rows = psql("select q,r from map_tiles where terrain in ('plains','hills','forest_olive_grove') and abs(q-8)<=3 and abs(r-57)<=3 order by (q-8)^2+(r-57)^2 desc").split()
    for row in rows:
        q, r = map(int, row.split('|'))
        st, _ = api(c, f"/api/v1/worlds/{c['world_id']}/units/{uid}/march-preview?target_q={q}&target_r={r}")
        if st == 200 and (q, r) != (8, 57): return q, r
    raise SystemExit('no reachable land hex found')


def item1(pw):
    c = cfg('Varv1')
    for w, h in SIZES:
        tag = f'{w}'
        print(f'item 1 @ {w}x{h}')
        reset_units('Varv1')
        # (b) a land unit sent at a lake
        b, p, _ = newpage(pw, 'Varv1', w, h)
        war_march_to(p, w, 11, 58)
        shot(p, 1, f'b-lake-open-{tag}')
        text = p.evaluate("document.getElementById('march-ctx').innerText")
        chk(1, f'b {tag}: soldiers aimed at a lake get the one plain sentence', 'Soldiers cannot cross water. They need a ship: build one in a coastal city.' in text, repr(text[:160]))
        chk(1, f'b {tag}: no sea-exploration text for soldiers', 'sail home' not in text and 'No galleys' not in text)
        p.click('#mctx-send'); p.wait_for_timeout(1500)
        err = p.evaluate("document.getElementById('mctx-err').innerText")
        units = p.evaluate("document.querySelectorAll('#mctx-units input[type=checkbox]').length")
        chk(1, f'b {tag}: pressing March never asks to choose forces from an empty list', not (('Choose the forces' in err) and units == 0), repr(err))
        v = in_view(p, '#mctx-err')
        chk(1, f'b {tag}: whatever March answered is inside the visible part of the dialog', v.get('visible') or not err, str(v))
        shot(p, 1, f'b-lake-march-{tag}')
        b.close()
        # (a) a refusal: explore toward a hex with no route
        b, p, _ = newpage(pw, 'Varv1', w, h)
        war_march_to(p, w, 38, 43)
        p.locator('#mctx-explore-chk').check(); p.wait_for_timeout(800)
        p.click('#mctx-send'); p.wait_for_timeout(2000)
        err = p.evaluate("document.getElementById('mctx-err').innerText")
        v = in_view(p, '#mctx-err')
        chk(1, f'a {tag}: the refusal exists ("Nothing sent — No route")', 'Nothing sent' in err, repr(err[:80]))
        chk(1, f'a {tag}: the refusal is inside the visible part of the dialog and the viewport', v.get('visible'), str(v))
        send = in_view(p, '#mctx-send')
        chk(1, f'a {tag}: it sits next to the March button', send.get('visible') and v.get('rect') and abs(v['rect'][1] - send['rect'][3]) < 120, f"send={send.get('rect')} err={v.get('rect')}")
        shot(p, 1, f'a-no-route-{tag}')
        b.close()
        # (c) success: on a phone the sheet closes back to the map and the confirmation is shown there
        q, r = reachable_land(c)
        reset_units('Varv1')
        b, p, _ = newpage(pw, 'Varv1', w, h)
        war_march_to(p, w, q, r)
        p.click('#mctx-send'); p.wait_for_timeout(2500)
        if w < 500:
            closed = p.evaluate("getComputedStyle(document.getElementById('march-ctx')).display") == 'none'
            note = in_view(p, '#map-notice')
            chk(1, f'c {tag}: the sheet is closed and the map is back', closed)
            chk(1, f'c {tag}: the confirmation is visible on the map ("Forces marching")', note.get('visible') and 'Forces marching' in note.get('text', ''), str(note))
        else:
            eta = in_view(p, '#mctx-eta')
            chk(1, f'c {tag}: desktop keeps the confirmation in the dialog, visible', eta.get('visible') and 'Forces marching' in eta.get('text', ''), str(eta))
        shot(p, 1, f'c-success-{tag}')
        b.close()


def give_stock(name, **goods):
    """Acceptance world only: put goods into the city store through psql (the start store has no timber or stone)."""
    for g, n in goods.items():
        psql(f"update settlement_goods set amount={n} where good_key='{g}' and settlement_id=(select s.id from settlements s join players p on p.id=s.owner_id where p.username='{name}' limit 1)")


def open_city(p, w, tab=None):
    nav(p, 'City', w)
    if tab: p.evaluate("t => document.querySelector(`#city-body [data-tab=\"${t}\"]`).click()", tab); p.wait_for_timeout(700)


def item2(pw):
    for w, h in SIZES:
        tag = f'{w}'
        print(f'item 2 @ {w}x{h}')
        psql("delete from build_queue where settlement_id=(select s.id from settlements s join players p on p.id=s.owner_id where p.username='Varv3')")
        psql("delete from buildings where settlement_id=(select s.id from settlements s join players p on p.id=s.owner_id where p.username='Varv3') and type='farm'")
        give_stock('Varv3', stone=100, timber=100)
        b, p, _ = newpage(pw, 'Varv3', w, h)
        open_city(p, w, 'byggnader')
        p.locator('#city-bld-sec .build-row[data-build-type="farm"] [data-build-act="pick"]').click(); p.wait_for_timeout(1800)
        v = in_view(p, '.gubbe-svg')
        chk(2, f'a {tag}: after "Choose a hex →" the picker (hex grid) is inside the visible panel', v.get('visible'), str(v))
        shot(p, 2, f'a-picker-{tag}')
        poly = p.locator('.gubbe-hex.pick-ok').first
        label = p.evaluate("document.querySelector('.gubbe-hex.pick-ok').parentElement.querySelector('.gubbe-hex-label').textContent")
        poly.click(); p.wait_for_timeout(900)
        row = in_view(p, '#gubbe-detail .build-row-btn')
        chk(2, f'a {tag}: the chosen hex shows its Build button inside the visible panel', row.get('visible'), str(row))
        # a refusal first: the store is emptied behind the open picker, then BUILD
        give_stock('Varv3', stone=0, timber=0)
        p.locator('#gubbe-detail .build-row-btn').click(); p.wait_for_timeout(2500)
        res = in_view(p, '.build-row-result, .gubbe-refusal')
        chk(2, f'b {tag}: a refusal is visible where the player is', res.get('visible'), str(res))
        shot(p, 2, f'b-refusal-{tag}')
        give_stock('Varv3', stone=100, timber=100)
        p.locator('#gubbe-detail .build-row-btn').click(); p.wait_for_timeout(3000)
        res = in_view(p, '.build-row-result, .gubbe-refusal')
        q, r = label.split(',')
        chk(2, f'b {tag}: the confirmation "Farm queued at ({label})" is visible after the re-render', res.get('visible') and 'Farm queued' in res.get('text', '') and f'({q},{r})' in res.get('text', ''), str(res))
        shot(p, 2, f'b-confirmation-{tag}')
        p.wait_for_timeout(4000)
        res2 = in_view(p, '.build-row-result, .gubbe-refusal')
        chk(2, f'b {tag}: it stays until the player acts again', res2.get('visible') and 'Farm queued' in res2.get('text', ''), str(res2))
        b.close()


ITEMS = {1: item1, 2: item2}


def main():
    args = sys.argv[1:]
    if args[:1] == ['setup']: return setup()
    want = [int(a) for a in args] or sorted(ITEMS)
    with sync_playwright() as pw:
        for i in want: ITEMS[i](pw)
    bad = [r for r in results if not r[2]]
    print(f'\n{len(results) - len(bad)} passed, {len(bad)} failed')
    for i, n, _ in bad: print('  FAILED item', i, n)
    sys.exit(1 if bad else 0)


if __name__ == '__main__':
    main()
