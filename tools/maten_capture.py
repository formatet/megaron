#!/usr/bin/env python3
"""Food figures on every web screen for the SAME city, real Firefox, isolated acceptance world (http://localhost:8097).
Usage: maten_capture.py LABEL [--shots DIR]  -> prints the food lines of City>Production, City>Warehouse, Economy;
with --shots also writes pictures at 1280 and 390 wide. Config: /tmp/claude-1000/kx-acc.json (see tools/acceptance.sh player)."""
import json, re, sys
from pathlib import Path
from playwright.sync_api import sync_playwright
CFG = json.load(open('/tmp/claude-1000/kx-acc.json')); BASE = 'http://localhost:8097'
WORLD, TOKEN = CFG['world_id'], CFG['token']
label = sys.argv[1]; shots = Path(sys.argv[sys.argv.index('--shots') + 1]) if '--shots' in sys.argv else None
FOOD = re.compile(r'food|grain|lasts|herd|ration|meal|fish|oil|growing|army', re.I)
def lines(page, sel):
    t = page.locator(sel).inner_text()
    ls = [l.strip() for l in t.splitlines() if l.strip()]
    keep = [i for i, l in enumerate(ls) if FOOD.search(l)]
    return [ls[i] for i in sorted({j for i in keep for j in (i, i + 1) if j < len(ls)})]
out = {}
with sync_playwright() as pw:
    b = pw.firefox.launch()
    for w, h in ((1280, 800), (390, 844)):
        ctx = b.new_context(viewport=dict(width=w, height=h), device_scale_factor=1)
        ctx.add_cookies([dict(name='poleia_token', value=TOKEN, url=BASE)])
        p = ctx.new_page(); p.goto(f'{BASE}/world/{WORLD}/map'); p.wait_for_timeout(3500)
        def shot(name):
            if shots: shots.mkdir(parents=True, exist_ok=True); p.screenshot(path=str(shots / f'{label}-{name}-{w}.png'))
        p.evaluate("window.openDrawer('city')"); p.wait_for_timeout(2500)
        out[f'{w}:production'] = lines(p, '#drawer-city'); shot('production')
        p.click('.dtab[data-tab="forrad"]'); p.wait_for_timeout(800)
        out[f'{w}:warehouse'] = lines(p, '#drawer-city'); shot('warehouse')
        p.evaluate("window.openDrawer('economy')"); p.wait_for_timeout(2500)
        out[f'{w}:economy'] = lines(p, '#drawer-economy'); shot('economy')
        ctx.close()
    b.close()
print(json.dumps(out, indent=1))
