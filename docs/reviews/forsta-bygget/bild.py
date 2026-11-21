#!/usr/bin/env python3
"""BILD for 'nobody makes stone' (order-forsta-bygget-20261118): Buildings tab + hex build row in a fresh
acceptance city (0 stone/timber/cedar), Firefox, 1:1, 1280 and 390 wide."""
import json
from pathlib import Path
from playwright.sync_api import sync_playwright
OUT = Path(__file__).parent
cfg = json.load(open('/tmp/claude-1000/kx-acc.json')); WORLD = cfg['world_id']; TOKEN = cfg['token']; BASE = 'http://localhost:8097'
for w, h, tag in ((1280, 800, 'desktop'), (390, 844, 'mobile')):
    with sync_playwright() as pw:
        b = pw.firefox.launch()
        ctx = b.new_context(viewport=dict(width=w, height=h), device_scale_factor=1, has_touch=(w < 500))
        ctx.add_cookies([dict(name='poleia_token', value=TOKEN, url=BASE)])
        p = ctx.new_page(); errs = []
        p.on('pageerror', lambda e: errs.append(str(e)))
        p.goto(f'{BASE}/world/{WORLD}/map'); p.wait_for_timeout(3500)
        p.evaluate("import('/static/js/megaron/state.js').then(m=>{const c=m.State.provinceData.find(x=>x.own);m.State.cityViewID=c.id;m.State.selectedHex={q:c.q,r:c.r};window.openDrawer('city')})")
        p.wait_for_timeout(2500)
        tab = lambda t: (p.click(f'#city-body [data-tab="{t}"]'), p.wait_for_timeout(1500))
        tab('byggnader')
        print(tag, 'list reasons:', p.evaluate("[...document.querySelectorAll('#city-bld-sec .build-row')].map(r=>r.textContent.replace(/\\s+/g,' ').trim()).filter(t=>t.includes('needs')).slice(0,4)"))
        p.evaluate("(document.querySelector('#city-bld-sec .build-row') || document.querySelector('#city-bld-sec')).scrollIntoView({block:'start'})")
        p.wait_for_timeout(400); p.screenshot(path=str(OUT / f'a-buildings-{tag}.png'))
        tab('produktion')
        for g in p.query_selector_all('.gubbe-hex-g[data-target="hex"]'):
            g.click(force=True); p.wait_for_timeout(300)
            if p.query_selector('#gubbe-detail .build-row'): break
        print(tag, 'hex row:', p.evaluate("(document.querySelector('#gubbe-detail .build-row')||{}).textContent?.replace(/\\s+/g,' ').trim()"))
        p.evaluate("document.querySelector('#gubbe-detail .build-row')?.scrollIntoView({block:'center'})")
        p.wait_for_timeout(400); p.screenshot(path=str(OUT / f'b-hexrow-{tag}.png'))
        print(tag, 'errors', errs); b.close()
