#!/usr/bin/env python3
"""BILD: activity stamp always on (no ⚒ button), 2x, own city building. Acceptance world, Firefox 1:1."""
import json
from pathlib import Path
from playwright.sync_api import sync_playwright
from PIL import Image
OUT = Path(__file__).parent
cfg = json.load(open('/tmp/claude-1000/kx-acc.json')); BASE = 'http://localhost:8097'
with sync_playwright() as pw:
    b = pw.firefox.launch()
    ctx = b.new_context(viewport=dict(width=1280, height=800), device_scale_factor=1)
    ctx.add_cookies([dict(name='poleia_token', value=cfg['token'], url=BASE)])
    p = ctx.new_page(); errs = []
    p.on('pageerror', lambda e: errs.append(str(e)))
    p.goto(f"{BASE}/world/{cfg['world_id']}/map"); p.wait_for_timeout(4500)
    print(p.evaluate("import('/static/js/megaron/state.js').then(m=>{const c=m.State.provinceData.find(x=>x.own);return {build:c.build_active,train:c.train_active,zoom:m.State.camera.zoom,btn:!!document.getElementById('activity-btn')}})"))
    p.screenshot(path=str(OUT / 'e1-stamp-building.png'))
    print('errors', errs); b.close()
im = Image.open(OUT / 'e1-stamp-building.png')
im.crop((140, 330, 260, 470)).resize((480, 560), Image.NEAREST).save(OUT / 'e2-stamp-building-zoom4x.png')
