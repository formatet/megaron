#!/usr/bin/env python3
"""BILD for order-oforklarade-extra-20261122: silver card, kharis line, hex row, and the ⚒ overlay at default zoom.
Acceptance world, Firefox, 1:1, 1280x800."""
import json
from pathlib import Path
from playwright.sync_api import sync_playwright
OUT = Path(__file__).parent
cfg = json.load(open('/tmp/claude-1000/kx-acc.json')); WORLD = cfg['world_id']; TOKEN = cfg['token']; BASE = 'http://localhost:8097'
with sync_playwright() as pw:
    b = pw.firefox.launch()
    ctx = b.new_context(viewport=dict(width=1280, height=800), device_scale_factor=1)
    ctx.add_cookies([dict(name='poleia_token', value=TOKEN, url=BASE)])
    p = ctx.new_page(); errs = []
    p.on('pageerror', lambda e: errs.append(str(e)))
    p.goto(f'{BASE}/world/{WORLD}/map'); p.wait_for_timeout(4000)
    # ⚒ at default zoom: centre own city, screenshot off, then on
    probe = "import('/static/js/megaron/state.js').then(m=>{const c=m.State.provinceData.find(x=>x.own);return {zoom:m.State.camera.zoom, own:!!c, outpost:c&&c.is_outpost, build:c&&c.build_active, train:c&&c.train_active, n:m.State.provinceData.length}})"
    print('probe', p.evaluate(probe))
    p.click('button[title^="Return"], #home-btn, .mzo-btn:has-text("⌂")'); p.wait_for_timeout(1200)
    p.screenshot(path=str(OUT / 'd1-hammer-off.png'))
    p.click('#activity-btn'); p.wait_for_timeout(1200)
    print('overlay on', p.evaluate("import('/static/js/megaron/state.js').then(m=>({on:m.State.activityOverlay, zoom:m.State.camera.zoom}))"))
    p.screenshot(path=str(OUT / 'd2-hammer-on.png'))
    p.click('#activity-btn'); p.wait_for_timeout(500)
    # economy silver card
    p.evaluate("toggleDrawer('economy')"); p.wait_for_timeout(2500)
    p.evaluate("document.querySelector('.silver-balance')?.scrollIntoView({block:'center'})"); p.wait_for_timeout(400)
    print('silver', p.evaluate("document.querySelector('.silver-balance')?.textContent.replace(/\\s+/g,' ').trim()"))
    p.screenshot(path=str(OUT / 'c-silver.png'))
    p.evaluate("closeDrawer('economy')"); p.wait_for_timeout(500)
    # city: kult tab kharis + hex row
    p.evaluate("import('/static/js/megaron/state.js').then(m=>{const c=m.State.provinceData.find(x=>x.own);m.State.cityViewID=c.id;m.State.selectedHex={q:c.q,r:c.r};window.openDrawer('city')})")
    p.wait_for_timeout(2500)
    p.click('#city-body [data-tab="produktion"]'); p.wait_for_timeout(1500)
    for g in p.query_selector_all('.gubbe-hex-g[data-target="hex"]'):
        g.click(force=True); p.wait_for_timeout(300)
        if 'on this hex' in (p.evaluate("document.querySelector('#gubbe-detail')?.textContent||''")): break
    print('hexrow', p.evaluate("[...document.querySelectorAll('#gubbe-detail .gubbe-good-line')].map(e=>e.textContent.replace(/\\s+/g,' ').trim())"))
    p.evaluate("document.querySelector('#gubbe-detail .gubbe-good-line')?.scrollIntoView({block:'center'})"); p.wait_for_timeout(400)
    p.screenshot(path=str(OUT / 'a-hexrow.png'))
    tabs = p.evaluate("[...document.querySelectorAll('#city-body [data-tab]')].map(e=>e.dataset.tab)")
    print('tabs', tabs)
    p.evaluate("toggleDrawer('kult')"); p.wait_for_timeout(2500)
    print('kharis', p.evaluate("[...document.querySelectorAll('*')].map(e=>e.childElementCount===0?e.textContent:'').filter(t=>/Kharis \\d/.test(t)).slice(0,3)"))
    p.screenshot(path=str(OUT / 'b-kharis.png'))
    print('errors', errs); b.close()
