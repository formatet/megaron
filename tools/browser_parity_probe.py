#!/usr/bin/env python3
"""Calibrate the actual parity DOM probe: clipping is not an occlusion;
visible occlusion and an unreachable primary action must still fail."""
import json
from browser_parity import MEASURE
from playwright.sync_api import sync_playwright
with sync_playwright() as pw:
 browser=pw.firefox.launch();page=browser.new_page(viewport={'width':390,'height':844})
 page.set_content('''<style>body{margin:0}.panel{position:absolute;top:30px;width:300px;height:300px}.header{height:80px;background:tan}.body{height:180px;overflow:auto}.spacer{height:400px}button{height:30px}</style><div class="panel"><div class="header">Header</div><div class="body"><button id="scrolled">A scrolled-away action</button><div class="spacer"></div><button id="shown">A visible action</button></div></div>''')
 page.locator('.body').evaluate('el=>el.scrollTop=50')
 m=page.evaluate(MEASURE,['.panel',None]);assert not m['covered'],f'scrolled-away control wrongly counted as covered: {m}'
 page.locator('#shown').scroll_into_view_if_needed()
 page.evaluate('''()=>{const e=document.querySelector('#shown'),r=e.getBoundingClientRect(),d=document.createElement('div');d.id='occluder';Object.assign(d.style,{position:'fixed',left:r.x+'px',top:r.y+'px',width:r.width+'px',height:r.height+'px',background:'red',zIndex:100});document.body.append(d)}''')
 m=page.evaluate(MEASURE,['.panel','#shown']);assert any('#shown' in c for c in m['covered']),f'actual occlusion missed: {m}';assert m.get('unreachable'),f'covered primary action missed: {m}'
 print('PASS: clipped content excluded; visible occlusion and primary action retained')
 browser.close()
