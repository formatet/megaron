#!/usr/bin/env python3
"""First-hour round two, item C: measure whether the +1 / -1 / Fill buttons of the worker grid move
(getBoundingClientRect, viewport coordinates) after a click on +1 and on -1.
Run against the isolated acceptance world with a founded city:
  python3 tools/varv2_knappar_matning.py CONFIG.json   (CONFIG: acceptance.sh player ... CONFIG)
Exit 1 when any button moved more than 1 px. Firefox, 1280x800 and 390x844."""
import json, sys
from playwright.sync_api import sync_playwright

cfg = json.load(open(sys.argv[1])); W = cfg['world_id']; T = cfg['token']; B = 'http://localhost:8097'
VERBS = ['place1', 'fill', 'unplace1']

def rects(p, good):
    return p.evaluate("""g => { const row = document.querySelector(`#gubbe-detail .gubbe-good-row[data-good="${g}"]`);
      if (!row) return null; const o = {};
      for (const v of ['place1','fill','unplace1']) { const b = row.querySelector(`button[data-verb="${v}"]`);
        if (b) { const r = b.getBoundingClientRect(); o[v] = [Math.round(r.left*10)/10, Math.round(r.top*10)/10, Math.round(r.width*10)/10, Math.round(r.height*10)/10]; } }
      return o; }""", good)

def run(pw, w, h):
    b = pw.firefox.launch()
    c = b.new_context(viewport=dict(width=w, height=h), has_touch=w < 500)
    c.add_cookies([dict(name='poleia_token', value=T, url=B)])
    p = c.new_page(); p.goto(f'{B}/world/{W}/map'); p.wait_for_timeout(4000)
    p.evaluate("document.querySelector('.game-nav .win-trigger').click()") if w < 500 else p.click('text=City')
    p.wait_for_timeout(2500)
    bad = 0
    # a hex with a grain row (plains) and one with timber
    for good in ('grain', 'timber', 'fish'):
        ords = p.evaluate("""g => [...document.querySelectorAll('.gubbe-hex-g[data-target="hex"]')].map(e => e.dataset.ordinal)""", good)
        for o in ords:
            p.click(f'.gubbe-hex-g[data-ordinal="{o}"]'); p.wait_for_timeout(300)
            if not rects(p, good): continue
            p.evaluate("document.querySelector('#gubbe-detail').scrollIntoView({block:'center'})"); p.wait_for_timeout(300)
            for verb in ('place1', 'unplace1'):
                before = rects(p, good)
                if not before or verb not in before: continue
                if p.evaluate("(a)=>document.querySelector(`#gubbe-detail .gubbe-good-row[data-good=\"${a[0]}\"] button[data-verb=\"${a[1]}\"]`).disabled", [good, verb]): continue
                x, y, bw, bh = before[verb]  # click by coordinates: no auto-scroll between the two measurements
                p.mouse.click(x + bw / 2, y + bh / 2); p.wait_for_timeout(1800)
                after = rects(p, good)
                moved = {k: (before[k], after.get(k)) for k in before if after is None or after.get(k) is None or max(abs(a - b2) for a, b2 in zip(before[k], after[k])) > 1}
                print(w, good, 'hex', o, verb, 'MOVED' if moved else 'stable', moved or '')
                bad += bool(moved)
            break
    b.close(); return bad

with sync_playwright() as pw:
    total = run(pw, 1280, 800) + run(pw, 390, 844)
sys.exit(1 if total else 0)
