#!/usr/bin/env python3
"""Inventory of which desktop windows may be open together, on the REAL canvas map in Firefox, against the isolated
acceptance world (tools/acceptance.sh up, http://localhost:8097). For every ordered pair (open A, then open B) at
1280x800 and 1920x1080 it records: do the visible boxes overlap, is A still open afterwards, is A open after B closes.
Usage: fonster_inventory.py [before|after]   Writes docs/reviews/fonster-ur-vagen/{table-<tag>.md,results-<tag>.json}
and a picture of each pair that overlaps (named <A>-then-<B>-<width>[-after].png)."""
from pathlib import Path
import json, os, subprocess, sys, time
from playwright.sync_api import sync_playwright

TAG = sys.argv[1] if len(sys.argv) > 1 else 'before'
SFX = '' if TAG == 'before' else '-after'
ROOT = Path(__file__).resolve().parents[1]
OUT = ROOT / 'docs/reviews/fonster-ur-vagen'; OUT.mkdir(parents=True, exist_ok=True)
ACC = str(ROOT / 'tools/acceptance.sh')
KX = '/tmp/claude-1000/kx-fonster'; CFG = '/tmp/claude-1000/kx-acc-fonster.json'
BASE = 'http://localhost:8097'
sh = lambda *a, **kw: subprocess.run(list(a), capture_output=True, text=True, **kw)
kx = lambda *a: sh(KX, *a, env={**os.environ, 'POLEIA_CONFIG': CFG})

if not os.path.exists(CFG):
    sh('go', 'build', '-o', KX, './cmd/keryx/', cwd=ROOT / 'server')
    sh(ACC, 'player', 'hp1', CFG)
    for n in ('hp2', 'hp3', 'hp4'): sh(ACC, 'player', n)
    time.sleep(2)
    if 'Metropolis' not in kx('dossier').stdout and 'capital' not in kx('dossier').stdout:
        print(kx('founding', 'settle', '--yes').stdout[-200:])
cfg = json.load(open(CFG)); WORLD = cfg['world_id']; TOKEN = cfg['token']

WINDOWS = ['hex', 'march', 'drawer-city', 'drawer-war', 'drawer-report', 'codex', 'dispatch', 'search', 'account']

JS_LIB = r'''
(() => {
 const $ = id => document.getElementById(id);
 const vis = e => e && getComputedStyle(e).display !== 'none' && getComputedStyle(e).visibility !== 'hidden';
 // visible box, with a drawer's slide-in transform taken out
 const box = e => {
   if (!e) return null;
   const inner = e.querySelector(':scope > .search-box, :scope > .dispatch-window');
   const t = inner || e; const r = t.getBoundingClientRect();
   let dx = 0; try { dx = new DOMMatrix(getComputedStyle(t).transform).m41; } catch (_) {}
   return {l: r.left - dx, t: r.top, r: r.right - dx, b: r.bottom, w: r.width, h: r.height};
 };
 const el = {hex: () => $('inspect-panel'), march: () => $('march-ctx') || document.querySelector('.march-ctx'),
   codex: () => $('codex-panel'), dispatch: () => $('dispatch-window-overlay'), search: () => $('search-overlay'),
   account: () => $('account-window-overlay')};
 const drawerOf = n => $('drawer-' + n.slice(7));
 const element = n => n.startsWith('drawer-') ? drawerOf(n) : el[n]();
 const State = async () => (await import('/static/js/megaron/state.js')).State;
 window.__inv = {
  box: n => box(element(n)),
  async isOpen(n) {
    const S = await State(); const e = element(n);
    if (n === 'hex') return !!S.selectedHex && e.style.display !== 'none';
    if (n === 'march') return e.style.display === 'block';
    if (n.startsWith('drawer-')) return S.activeDrawer === n.slice(7);
    return e.classList.contains('open');
  },
  closeAll: async () => {
    const S = await State();
    window.closeInspect(); window.closeMarchCtx(); window.closeCodex(); window.closeDispatchWindow(); window.closeSearch(); window.closeAccountWindow();
    if (S.activeDrawer) window.closeDrawer(S.activeDrawer);
  },
  close: async n => {
    const S = await State();
    ({hex: () => window.closeInspect(), march: () => window.closeMarchCtx(), codex: () => window.closeCodex(),
      dispatch: () => window.closeDispatchWindow(), search: () => window.closeSearch(), account: () => window.closeAccountWindow()})[n]?.();
    if (n.startsWith('drawer-') && S.activeDrawer === n.slice(7)) window.closeDrawer(n.slice(7));
  },
  openJS: async (n, hexA, hexB, pos) => {
    if (n === 'codex') return window.openCodex();
    if (n === 'dispatch') { const m = await import('/static/js/megaron/ui/dispatch_window.js'); return m.openDispatchWindow('OrderFailed', {}, 'now'); }
    if (n === 'search') return window.toggleSearch();
    if (n === 'account') return window.toggleAccountWindow();
    if (n.startsWith('drawer-')) return window.toggleDrawer(n.slice(7));
  },
 };
})()
'''

def main():
    rows = []
    with sync_playwright() as pw:
        b = pw.firefox.launch()
        for (W, H) in ((1280, 800), (1920, 1080)):
            ctx = b.new_context(viewport=dict(width=W, height=H), device_scale_factor=1)
            ctx.add_cookies([dict(name='poleia_token', value=TOKEN, url=BASE)])
            p = ctx.new_page(); errs = []
            p.on('pageerror', lambda e: errs.append(str(e)))
            p.goto(f'{BASE}/world/{WORLD}/map'); p.wait_for_timeout(3500)
            p.wait_for_function("import('/static/js/megaron/state.js').then(m=>m.State.provinceData&&m.State.provinceData.length>=1)")
            p.evaluate(JS_LIB)
            ev = p.evaluate
            S = lambda js: ev("import('/static/js/megaron/state.js').then(m=>" + js + ")")
            M = lambda js: ev("import('/static/js/megaron/render/map.js').then(m=>" + js + ")")
            cq, cr = S("m.State.provinceData.filter(x=>x.own).map(x=>[x.q,x.r])[0]")
            adj = S(f"""m.State.tileData.filter(t=>!['fog','coastal_sea','deep_sea','mountain_limestone','mountain_red','river','lake'].includes(t.terrain)
              &&Math.max(Math.abs(t.q-{cq}),Math.abs(t.r-{cr}),Math.abs(t.q+t.r-{cq}-{cr}))===1&&!m.State.provinceData.some(p=>p.q===t.q&&p.r===t.r)).map(t=>[t.q,t.r])""")
            hx = M(f"m.hexClientPos({adj[0][0]},{adj[0][1]})")
            # a second land hex, well clear of the right edge, for the march menu (right-click)
            land = S("m.State.tileData.filter(t=>!['fog','coastal_sea','deep_sea','mountain_limestone','mountain_red','river','lake'].includes(t.terrain)).map(t=>[t.q,t.r])")
            cands = [(q, r, M(f"m.hexClientPos({q},{r})")) for q, r in land[:400]]
            cands = [c for c in cands if 60 < c[2]['x'] < W * 0.8 and 120 < c[2]['y'] < H - 120 and abs(c[2]['x'] - hx['x']) > 80]
            cands.sort(key=lambda c: abs(c[2]['x'] - W * 0.58))
            mp = cands[0][2]
            print(W, 'hex px', hx, 'march px', mp, flush=True)
            def open_(n):
                if n == 'hex': p.mouse.click(hx['x'], hx['y'])
                elif n == 'march': p.mouse.click(mp['x'], mp['y'], button='right')
                else: ev(f"window.__inv.openJS({json.dumps(n)})")
                p.wait_for_timeout(550)
            for a in WINDOWS:
                for bb in WINDOWS:
                    if a == bb: continue
                    ev("window.__inv.closeAll()"); p.wait_for_timeout(250)
                    open_(a)
                    if not ev(f"window.__inv.isOpen({json.dumps(a)})"):
                        rows.append(dict(w=W, a=a, b=bb, note='A did not open')); print('A did not open', a, flush=True); continue
                    boxA = ev(f"window.__inv.box({json.dumps(a)})")
                    open_(bb)
                    if not ev(f"window.__inv.isOpen({json.dumps(bb)})"):
                        rows.append(dict(w=W, a=a, b=bb, note='B did not open')); print('B did not open', bb, flush=True); continue
                    boxB = ev(f"window.__inv.box({json.dumps(bb)})")
                    ov = boxA and boxB and boxA['w'] > 0 and boxB['w'] > 0 and not (boxA['r'] <= boxB['l'] or boxB['r'] <= boxA['l'] or boxA['b'] <= boxB['t'] or boxB['b'] <= boxA['t'])
                    ov = bool(ov)
                    a_open = ev(f"window.__inv.isOpen({json.dumps(a)})")
                    if ov and a_open:
                        p.screenshot(path=str(OUT / f'{a}-then-{bb}-{W}{SFX}.png'))
                    ev(f"window.__inv.close({json.dumps(bb)})"); p.wait_for_timeout(350)
                    a_open_after = ev(f"window.__inv.isOpen({json.dumps(a)})")
                    r = dict(w=W, a=a, b=bb, overlap=ov, a_open_with_b=a_open, a_open_after_b_closed=a_open_after, boxA=boxA, boxB=boxB)
                    rows.append(r); print(r['w'], a, '->', bb, 'overlap', ov, 'A open', a_open, 'A after B closed', a_open_after, flush=True)
            ev("window.__inv.closeAll()")
            if errs: print('PAGE ERRORS', errs[:5])
            ctx.close()
        b.close()
    (OUT / f'results-{TAG}.json').write_text(json.dumps(rows, indent=1))
    lines = [f'# Pair table ({TAG})', '', 'Open A, then open B. overlap = visible boxes intersect. **PROBLEM** = overlap and A still open (A covered, not closed). Reappears = A open again after B closed, having been covered.', '']
    for W in (1280, 1920):
        lines += [f'## {W} px', '', '| A (first) | B (then) | overlap | A open with B | A open after B closed | verdict |', '|---|---|---|---|---|---|']
        for r in rows:
            if r['w'] != W: continue
            if 'note' in r: lines.append(f"| {r['a']} | {r['b']} | - | - | - | {r['note']} |"); continue
            v = 'PROBLEM (covered, still open)' if r['overlap'] and r['a_open_with_b'] else ('ok: closed' if r['overlap'] else ('ok: apart, both stay' if r['a_open_with_b'] else 'A closed without overlap'))
            lines.append(f"| {r['a']} | {r['b']} | {'yes' if r['overlap'] else 'no'} | {'yes' if r['a_open_with_b'] else 'no'} | {'yes' if r['a_open_after_b_closed'] else 'no'} | {v} |")
        lines.append('')
    (OUT / f'table-{TAG}.md').write_text('\n'.join(lines))
main()
