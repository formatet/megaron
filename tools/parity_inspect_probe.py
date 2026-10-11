#!/usr/bin/env python3
"""Actual river panel text rectangles through the normal parity controller."""
import json,pathlib
import browser_parity as b
from playwright.sync_api import sync_playwright
out=pathlib.Path('docs/reviews/paritet-20261011/inspect-probe');out.mkdir(parents=True,exist_ok=True)
b.MEASURE=b.MEASURE.replace('const sticking =', '''out.inspect_text=[...box.querySelectorAll('.inspect-row')].map(row=>({text:row.textContent,spans:[...row.children].map(el=>{const r=el.getBoundingClientRect(),s=getComputedStyle(el),range=document.createRange();range.selectNodeContents(el);return {text:el.textContent,x:r.x,w:r.width,shrink:s.flexShrink,text_rects:[...range.getClientRects()].map(r=>({x:r.x,y:r.y,w:r.width,h:r.height}))}})}));const sticking =''')
with sync_playwright() as pw:
 rows=[]
 for engine in ['firefox','chromium','webkit']:
  rows+=b.run_engine(pw,engine,b.login('Eirene'),out,{'river':b.SURFACES['river']})
 (out/'rects.json').write_text(json.dumps(rows,indent=2)+'\n')
 for row in rows:
  for tr in row['inspect_text']:
   if tr['text'].strip().lower().startswith('produces'):print(row['engine'],row['mode'],json.dumps(tr),flush=True)
