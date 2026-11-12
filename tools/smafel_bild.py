#!/usr/bin/env python3
"""BILD for the first-hour small faults, Firefox, against the isolated acceptance world (tools/acceptance.sh up,
http://localhost:8097) with a founded city (config from tools/telefon_halvpanel_bild.py's pattern in /tmp/claude-1000/kx-acc.json).
Writes docs/reviews/smafel-forsta-timmen/."""
from pathlib import Path
import json
from playwright.sync_api import sync_playwright

ROOT = Path(__file__).resolve().parents[1]
OUT = ROOT / 'docs/reviews/smafel-forsta-timmen'; OUT.mkdir(parents=True, exist_ok=True)
cfg = json.load(open('/tmp/claude-1000/kx-acc.json')); W = cfg['world_id']; T = cfg['token']; B = 'http://localhost:8097'
UNIT = '48f7cb0a-87c5-4fe0-9e8b-a62f523178ce'

def page(b, w, h):
    c = b.new_context(viewport=dict(width=w, height=h), device_scale_factor=1, has_touch=w < 500)
    c.add_cookies([dict(name='poleia_token', value=T, url=B)])
    p = c.new_page(); p.goto(f'{B}/world/{W}/map'); p.wait_for_timeout(4000); return p

def shot(p, name): p.wait_for_timeout(400); p.screenshot(path=str(OUT / name)); print(name)

with sync_playwright() as pw:
    b = pw.firefox.launch()
    p = page(b, 1280, 900)
    shot(p, '10-chips-desktop.png')
    p.click('text=City'); p.wait_for_timeout(2500)
    shot(p, '09-walls-hold-citizens.png')
    p.click('.gubbe-hex-g[data-ordinal="16"]'); p.wait_for_timeout(800)
    p.click('#gubbe-detail .gubbe-good-row[data-good="grain"] button[data-verb="place1"]'); p.wait_for_timeout(1500)
    p.evaluate("document.querySelector('.stat-row#city-free-to-work').scrollIntoView()")
    shot(p, '01-free-count-agrees.png')
    p.evaluate("document.querySelector('#gubbe-detail').scrollIntoView()")
    shot(p, '02-hex-names-its-workers.png')
    p.click('#gubbe-detail .gubbe-good-row[data-good="grain"] button[data-verb="unplace1"]'); p.wait_for_timeout(1500)
    p.evaluate("[...document.querySelectorAll('.dtab')].find(e=>/warehouse/i.test(e.textContent))?.click()"); p.wait_for_timeout(800)
    shot(p, '07-warehouse-neutral-numbers.png')
    p.evaluate("document.querySelector('.drawer-close,.drawer-x,#drawer-city .close')?.click()")
    p.click('.game-nav >> text=Kult'); p.wait_for_timeout(2000)
    shot(p, '07-kult-neutral-numbers.png')
    p.keyboard.press('Escape')
    p.click('.game-nav >> text=War'); p.wait_for_timeout(2000)
    shot(p, '11-war-forces-stance.png')
    # 06: tooltip on a sea hex
    p2 = page(b, 1280, 900)
    pos = p2.evaluate("""Promise.all([import('/static/js/megaron/state.js'),import('/static/js/megaron/render/map.js')]).then(([s,m])=>{
      const t=s.State.tileData.find(t=>t.terrain==='coastal_sea')||s.State.tileData.find(t=>t.terrain==='deep_sea')||s.State.tileData.find(t=>t.terrain==='lake'); if(!t) return null; const c=m.hexClientPos(t.q,t.r); return [c.x,c.y]})""")
    if pos:
        p2.mouse.move(pos[0] - 3, pos[1] - 3); p2.mouse.move(pos[0], pos[1]); p2.wait_for_timeout(600)
        shot(p2, '06-sea-tooltip.png')
    else: print('no sea tile known in the acceptance world: 06 not pictured')
    b.close()
    b = pw.firefox.launch()
    q = page(b, 390, 844)
    shot(q, '10-chips-phone.png')
    q.evaluate("import('/static/js/megaron/ui/marchctx.js').then(m=>m.openMarchCtx({q:1,r:48,terrain:'plains',known:false,isSea:false},60,120))"); q.wait_for_timeout(3000)
    shot(q, '05-phone-duration-field-opened.png')
    q.evaluate("import('/static/js/megaron/ui/marchctx.js').then(m=>m.closeMarchCtx())")
    q.evaluate("document.querySelector('.game-nav .win-trigger').click()"); q.wait_for_timeout(2500)
    q.click('.gubbe-hex-g[data-ordinal="16"]'); q.wait_for_timeout(800)
    q.evaluate("document.querySelector('#gubbe-detail').scrollIntoView()")
    shot(q, '12-phone-grid-before-tap.png')
    q.locator('#gubbe-detail .gubbe-good-row[data-good="oil"] button[data-verb="place1"]').click(); q.wait_for_timeout(1500)
    shot(q, '12-phone-grid-after-tap.png')
    q.locator('#gubbe-detail .gubbe-good-row[data-good="oil"] button[data-verb="unplace1"]').click(); q.wait_for_timeout(1500)
    b.close()
