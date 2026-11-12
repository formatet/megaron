#!/usr/bin/env python3
"""BILD for byggvagran-syns against the isolated acceptance world: 'poor' (0 stone/timber) or 'rich' [desktop|mobile]. Setup: see README in docs/reviews/byggvagran-syns/."""
import json, subprocess, sys
from pathlib import Path
from playwright.sync_api import sync_playwright
ROOT = Path('/home/tk/Projects/megaron-wt-byggvagran'); OUT = ROOT / 'docs/reviews/byggvagran-syns'
cfg = json.load(open('/tmp/claude-1000/kx-acc.json')); WORLD = cfg['world_id']; TOKEN = cfg['token']; BASE = 'http://localhost:8097'
def psql(q): return subprocess.run([str(ROOT/'tools/acceptance.sh'), 'psql', q], capture_output=True, text=True).stdout.strip()
CITY = psql(f"select id from settlements where owner_id='{cfg['player_id']}' limit 1")
def setgoods(stone, timber):
    for g, v in (('stone', stone), ('timber', timber)):
        psql(f"update settlement_goods set amount={v}, rate=0, calc_tick=current_world_tick() where settlement_id='{CITY}' and good_key='{g}'")
which = sys.argv[1]  # 'poor' or 'rich'
def run(w, h, tag):
    with sync_playwright() as pw:
        b = pw.firefox.launch()
        ctx = b.new_context(viewport=dict(width=w, height=h), device_scale_factor=1, has_touch=(w < 500))
        ctx.add_cookies([dict(name='poleia_token', value=TOKEN, url=BASE)])
        p = ctx.new_page(); errs = []
        p.on('pageerror', lambda e: errs.append(str(e)))
        p.goto(f'{BASE}/world/{WORLD}/map'); p.wait_for_timeout(3500)
        p.evaluate("import('/static/js/megaron/state.js').then(m=>{const c=m.State.provinceData.find(x=>x.own);m.State.cityViewID=c.id;m.State.selectedHex={q:c.q,r:c.r};window.openDrawer('city')})")
        p.wait_for_timeout(2500)
        shot = lambda n: (p.wait_for_timeout(400), p.screenshot(path=str(OUT / f'{n}-{tag}.png')))
        tab = lambda t: (p.click(f'#city-body [data-tab="{t}"]'), p.wait_for_timeout(1500))
        def scrollres():
            return p.evaluate("(()=>{const e=[...document.querySelectorAll('.build-row-result')].find(x=>x.getBoundingClientRect().height>0);if(!e)return null;const r=e.getBoundingClientRect();return {text:e.textContent,top:r.top,bottom:r.bottom,vh:innerHeight,sideways:document.documentElement.scrollWidth>innerWidth}})()")
        if which == 'poor':
            tab('produktion')
            groups = p.query_selector_all('.gubbe-hex-g[data-target="hex"]'); found = None
            for g in groups:
                g.click(force=True); p.wait_for_timeout(300)
                if p.query_selector('#gubbe-detail .build-row'): found = g; break
            print(tag, 'hex with build rows', bool(found), p.evaluate("document.querySelectorAll('#gubbe-detail .build-row').length"))
            p.evaluate("document.querySelector('#gubbe-detail .build-row').scrollIntoView({block:'start'})"); shot('a-buildhere')
            # simulate stale stock: enable the first button and click it, the server refuses
            p.evaluate("document.querySelector('#gubbe-detail .build-row-btn').disabled=false")
            p.click('#gubbe-detail .build-row-btn'); p.wait_for_timeout(1500)
            print(tag, 'b', scrollres()); shot('b-refusal-hex')
            tab('byggnader')
            p.evaluate("document.querySelector('#city-bld-sec .build-row-btn[data-build-act=build]').scrollIntoView({block:'center'})"); shot('c0-buildings-list')
            p.evaluate("document.querySelector('#city-bld-sec .build-row-btn[data-build-act=build]').disabled=false")
            p.click('#city-bld-sec .build-row-btn[data-build-act=build]'); p.wait_for_timeout(1500)
            print(tag, 'c', scrollres()); shot('c-refusal-buildings')
            tab('forrad'); p.evaluate("[...document.querySelectorAll('#city-wh-sec .dsec-title')].pop().scrollIntoView({block:'start'})"); shot('d-warehouse')
        else:
            tab('byggnader')
            btn = p.query_selector('#city-bld-sec .build-row-btn[data-build-act=build]:not([disabled])')
            print(tag, 'enabled build button', bool(btn))
            p.evaluate("document.querySelector('#city-bld-sec .build-row-btn[data-build-act=build]:not([disabled])').scrollIntoView({block:'center'})"); shot('e0-affordable')
            p.click('#city-bld-sec .build-row-btn[data-build-act=build]:not([disabled])'); p.wait_for_timeout(2000)
            print(tag, 'e', scrollres()); shot('e-success')
        print(tag, 'errors', errs); b.close()
for w, h, t in [x for x in ((1280, 800, 'desktop'), (390, 844, 'mobile')) if len(sys.argv) < 3 or sys.argv[2] == x[2]]:
    run(w, h, t)
