#!/usr/bin/env python3
"""Capture frozen production city scenes at native canvas size (320x152).

Usage: python3 tools/city_scene_cases.py OUT [HOST] [BEFORE_CITY_JS]
The optional old source is served through a browser route for same-fixture
before/after comparison. This is renderer proof, not server/player acceptance.
"""
import json
from pathlib import Path
import sys
from playwright.sync_api import sync_playwright

out = Path(sys.argv[1])
out.mkdir(parents=True, exist_ok=True)
host = sys.argv[2] if len(sys.argv) > 2 else 'http://127.0.0.1:18199'
old = Path(sys.argv[3]) if len(sys.argv) > 3 else None
kinds = ['farm', 'olive_press', 'winery', 'market', 'lumbermill',
         'stonequarry', 'mine', 'foundry', 'barracks', 'stable', 'temple']
cases = [
    ('new', 101, 0, [], []),
    ('small', 1000, 1, [{'type': t, 'level': 1} for t in ['farm', 'market', 'barracks']], []),
    ('equipped', 12000, 2, [{'type': t, 'level': 1} for t in kinds], []),
    ('upgraded', 28000, 3, [{'type': t, 'level': 3} for t in kinds], []),
    ('construction', 3200, 1, [{'type': t, 'level': 1} for t in ['farm', 'market']],
     [{'type': 'foundry', 'created_at': '2026-01-01T00:00:00Z', 'complete_at': '2026-01-01T00:10:00Z'}]),
]
errors = []
with sync_playwright() as p:
    browser = p.chromium.launch()
    for label, pop, walls, buildings, queue in cases:
        page = browser.new_page(viewport={'width': 740, 'height': 760}, device_scale_factor=1)
        page.on('pageerror', lambda e: errors.append(str(e)))
        page.add_init_script('Date.now=()=>Date.parse("2026-01-01T00:05:00Z")')
        if old:
            page.route('**/static/js/megaron/render/city.js',
                       lambda r: r.fulfill(content_type='application/javascript', body=old.read_text()))
        page.goto(host+'/static/showcase-world.html?zoom=1', wait_until='networkidle')
        page.evaluate('''()=>{
          document.body.innerHTML='<canvas id="case" style="width:320px;height:152px;image-rendering:pixelated"></canvas>';
          window.requestAnimationFrame=fn=>{window.sceneFrame=fn;return 0;};
          window.cancelAnimationFrame=()=>{};
        }''')
        page.evaluate('''async data=>{
          const C=await import('/static/js/megaron/render/city.js');
          C.startCityAnim(document.getElementById('case'),{terrain:'plains'},data.buildings,data.queue,
            {population:data.pop,walls:data.walls});
          window.sceneFrame(0); C.stopCityAnim();
        }''', {'pop': pop, 'walls': walls, 'buildings': buildings, 'queue': queue})
        page.locator('#case').screenshot(path=str(out/(label+'.png')))
        page.close()
    browser.close()
(out/'metadata.json').write_text(json.dumps({'cases': [c[0] for c in cases],
    'canvas': [320,152], 'css': [320,152], 'dpr': 1, 'fixed_time': '2026-01-01T00:05:00Z',
    'errors': errors, 'server_acceptance': False}, indent=2)+'\n')
print(json.dumps({'output': str(out), 'errors': errors}))
if errors:
    sys.exit(1)
