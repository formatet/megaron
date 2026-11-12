#!/usr/bin/env python3
"""BILD for order-fore-grundningen: what a newcomer sees BEFORE founding. Firefox, isolated acceptance world
(tools/acceptance.sh up + `player NAME /tmp/claude-1000/kx-fg.json`), one registered player who does NOT found.
usage: python3 tools/fore_bild.py TAG   (TAG = before | after; files land in docs/reviews/fore-grundningen/)"""
import sys, json
from pathlib import Path
from playwright.sync_api import sync_playwright

TAG = sys.argv[1]
ROOT = Path(__file__).resolve().parents[1]
OUT = ROOT / 'docs/reviews/fore-grundningen'; OUT.mkdir(parents=True, exist_ok=True)
cfg = json.load(open(sys.argv[3] if len(sys.argv) > 3 else "/tmp/claude-1000/kx-fg.json")); W = cfg['world_id']; T = cfg['token']; B = 'http://localhost:8097'

def page(b, w, h, path=None):
    c = b.new_context(viewport=dict(width=w, height=h), device_scale_factor=1, has_touch=w < 500)
    c.add_cookies([dict(name='poleia_token', value=T, url=B)])
    c.add_init_script(f"try{{localStorage.setItem('poleia_token','{T}')}}catch(e){{}}")
    p = c.new_page(); p.goto(f'{B}{path or f"/world/{W}/map"}'); p.wait_for_timeout(4000); return p

def shot(p, name):
    p.wait_for_timeout(500); p.screenshot(path=str(OUT / f'{TAG}-{name}.png')); print(name)

def select_host(p):
    pos = p.evaluate("""Promise.all([import('/static/js/megaron/state.js'),import('/static/js/megaron/render/map.js')]).then(([s,m])=>{
      const u=(s.State.unitsData||[]).find(u=>/host/i.test(u.type||u.unit_type||'')); if(!u) return null;
      const c=m.hexClientPos(u.q,u.r); return [c.x,c.y]})""")
    if pos: p.mouse.click(pos[0], pos[1]); p.wait_for_timeout(2500)
    return pos

with sync_playwright() as pw:
    b = pw.firefox.launch()
    for (w, h, tagw) in ((1280, 900, '1280'), (390, 844, '390')):
        p = page(b, w, h)
        shot(p, f'map-{tagw}')
        if select_host(p) is None: print('host not found')
        shot(p, f'host-panel-{tagw}')
        if tagw == '1280':
            p.evaluate("document.querySelector('#legend-btn')?.click()"); p.wait_for_timeout(600)
            shot(p, f'legend-{tagw}')
            p.keyboard.press('Escape')
        else:
            p.evaluate("document.querySelector('#legend-btn')?.click()"); p.wait_for_timeout(600)
            shot(p, f'legend-{tagw}')
            p.keyboard.press('Escape'); p.evaluate("window.closeLegendWindow&&window.closeLegendWindow()")
        for name, sel in (('city', 'City'), ('economy', 'Economy'), ('war', 'War'), ('report', 'Report'), ('feedback', 'Feedback')):
            try:
                p.evaluate("[...document.querySelectorAll('.drawer.open .drawer-close')].forEach(e=>e.click())")
                p.evaluate("(t)=>[...document.querySelectorAll('.game-nav .win-trigger')].find(e=>e.textContent.trim().endsWith(t))?.click()", sel)
                p.wait_for_timeout(1500)
                if p.evaluate("!!document.querySelector('.drawer.open')"): shot(p, f'drawer-{name}-{tagw}')
            except Exception as e: print(name, e)
        p.evaluate("[...document.querySelectorAll('.drawer.open .drawer-close')].forEach(e=>e.click())")
        p.evaluate("window.toggleAccountWindow&&window.toggleAccountWindow()"); p.wait_for_timeout(1500)
        shot(p, f'account-{tagw}')
        p.close()
    # dispatch: the notification archive (opened for the player whose config is CFG2: before = an old stored
    # dispatch, after = one made by the new code)
    cfg2 = json.load(open(sys.argv[2])) if len(sys.argv) > 2 else cfg
    T = cfg2['token']
    for (w, h, tagw) in ((1280, 900, '1280'), (390, 844, '390')):
        p = page(b, w, h)
        p.evaluate("window.toggleDrawer('notif')"); p.wait_for_timeout(2500)
        p.click('text=Your people await you'); p.wait_for_timeout(1500); shot(p, f'dispatch-{tagw}')
        p.close()
    # world entry card: an account that has not joined
    import urllib.request, time
    req = urllib.request.Request(f'{B}/api/v1/auth/register', method='POST', headers={'Content-Type': 'application/json'},
        data=json.dumps(dict(username=f'Gast{int(time.time())}', email=f'g{int(time.time())}@acceptance.local', password='acceptance-pw-123')).encode())
    reg = json.load(urllib.request.urlopen(req)); T = reg.get('access_token') or reg['token']
    p = page(b, 1280, 900, f'/world/{W}/join'); shot(p, 'join-1280')
    b.close()
