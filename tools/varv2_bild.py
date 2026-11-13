#!/usr/bin/env python3
"""Pictures for first-hour round two (Firefox) against the isolated acceptance world with a founded city.
  python3 tools/varv2_bild.py CONFIG.json     -> docs/reviews/forsta-timmen-varv2/"""
import json, sys
from pathlib import Path
from playwright.sync_api import sync_playwright
ROOT = Path(__file__).resolve().parents[1]; OUT = ROOT / 'docs/reviews/forsta-timmen-varv2'; OUT.mkdir(parents=True, exist_ok=True)
cfg = json.load(open(sys.argv[1])); W = cfg['world_id']; T = cfg['token']; B = 'http://localhost:8097'

def page(b, w, h):
    c = b.new_context(viewport=dict(width=w, height=h), has_touch=w < 500)
    c.add_cookies([dict(name='poleia_token', value=T, url=B)])
    p = c.new_page(); p.goto(f'{B}/world/{W}/map'); p.wait_for_timeout(3500); return p
def city(p, w):
    p.evaluate("document.querySelector('.game-nav .win-trigger').click()") if w < 500 else p.click('text=City'); p.wait_for_timeout(2500)
def shot(p, name): p.wait_for_timeout(400); p.screenshot(path=str(OUT / name)); print(name)
def hex_with(p, good):
    for o in p.evaluate("[...document.querySelectorAll('.gubbe-hex-g[data-target=\"hex\"]')].map(e=>e.dataset.ordinal)"):
        p.click(f'.gubbe-hex-g[data-ordinal="{o}"]'); p.wait_for_timeout(250)
        if p.query_selector(f'#gubbe-detail .gubbe-good-row[data-good="{good}"] .gubbe-places-note'): return o
with sync_playwright() as pw:
    b = pw.firefox.launch()
    for w, h in ((1280, 900), (390, 844)):
        p = page(b, w, h); city(p, w); tag = 'desktop' if w > 500 else 'phone'
        if hex_with(p, 'timber'):
            p.evaluate("document.querySelector('#gubbe-detail').scrollIntoView()"); shot(p, f'A-timber-places-{tag}.png')
        p.evaluate("document.querySelector('#gubbe-svg, .gubbe-svg').scrollIntoView()"); shot(p, f'R-hex-labels-{tag}.png')
        p.evaluate("[...document.querySelectorAll('.dtab')].find(e=>/buildings/i.test(e.textContent))?.click()"); p.wait_for_timeout(1200)
        p.evaluate("document.querySelector('.build-list')?.scrollIntoView()"); shot(p, f'D-choose-a-hex-disabled-{tag}.png')
        p.evaluate("[...document.querySelectorAll('.dtab')].find(e=>/warehouse/i.test(e.textContent))?.click()"); p.wait_for_timeout(800); shot(p, f'F-warehouse-{tag}.png')
        p.evaluate("[...document.querySelectorAll('.dtab')].find(e=>/production/i.test(e.textContent))?.click()"); p.wait_for_timeout(500)
        p.evaluate("document.querySelector('#city-pop-sec').scrollIntoView()"); shot(p, f'J-population-{tag}.png')
        if w > 500:
            p.keyboard.press('Escape'); p.click('.game-nav >> text=Kult'); p.wait_for_timeout(2000)
            p.evaluate("[...document.querySelectorAll('[data-cult-tab]')].find(e=>/temples/i.test(e.textContent))?.click()"); shot(p, 'L-kult-temples.png')
    b.close()
