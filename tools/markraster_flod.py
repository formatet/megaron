#!/usr/bin/env python3
"""Firefox proof at zoom 1.0; frozen baseline, closeups, determinism and containment.
Run from the worktree: python3 tools/markraster_flod.py --host http://localhost:8742
Only synthetic showcase fixtures are used; no backend or live-world claims.
"""
import argparse
import json
import subprocess
from pathlib import Path
from playwright.sync_api import sync_playwright
from PIL import Image, ImageChops
p = argparse.ArgumentParser()
p.add_argument('--host', default='http://localhost:8742')
a = p.parse_args()
out = Path('docs/reviews/markraster-flod'); out.mkdir(parents=True, exist_ok=True)
baseline = subprocess.check_output(['git','show','0d149589:web/static/js/megaron/render/map.js']).decode()
results = {}
with sync_playwright() as pw:
    browser = pw.firefox.launch()
    def shot(label, rig, query='', old=False):
        page = browser.new_page(viewport={'width':1600 if rig == 'world' else 1040, 'height':900}, device_scale_factor=1)
        errors = []
        page.on('pageerror', lambda e: errors.append(str(e)))
        page.on('console', lambda m: errors.append(m.text) if m.type == 'error' else None)
        if old:
            page.route('**/js/megaron/render/map.js', lambda route: route.fulfill(status=200, content_type='text/javascript', body=baseline))
        page.goto(a.host + f'/static/showcase-{rig}.html?' + query, wait_until='networkidle')
        page.wait_for_function('window.SHOWCASE?.ready', timeout=10000)
        page.evaluate('SHOWCASE.draw()')
        page.locator('#map-root').screenshot(path=str(out / f'{label}.png'))
        page.close()
        assert not errors, errors
    def diff(left,right):
        im = ImageChops.difference(Image.open(out/f'{left}.png').convert('RGB'),Image.open(out/f'{right}.png').convert('RGB'))
        return sum(1 for rgb in im.getdata() if rgb != (0,0,0))
    for label, old in [('before',True),('after',False),('repeat',False)]:
        shot(label, 'forest', 'fixture=river&zoom=1.0', old)
        im = Image.open(out/f'{label}.png')
        im.crop((120,340,420,530)).resize((900,570),Image.Resampling.NEAREST).save(out/f'{label}-closeup.png')
    results['river_determinism_changed_pixels'] = diff('after','repeat')
    assert results['river_determinism_changed_pixels'] == 0
    results['river_fix_changed_pixels'] = diff('before','after')
    assert results['river_fix_changed_pixels'] > 0
    for label, rig, query in [('world','world',''),('forest','forest','zoom=1.0'),('relief','forest','fixture=relief&zoom=1.0'),('cities','cities','')]:
        shot(label+'-before',rig,query,True)
        shot(label+'-after',rig,query)
        results[label+'_changed_pixels'] = diff(label+'-before',label+'-after')
        assert results[label+'_changed_pixels'] == 0, (label,results)
    for zoom in ['0.3','1.6']:
        shot('river-zoom-'+zoom,'forest','fixture=river&zoom='+zoom)
    browser.close()
(out/'browser.json').write_text(json.dumps(results,indent=2)+'\n')
print(json.dumps(results,indent=2))
