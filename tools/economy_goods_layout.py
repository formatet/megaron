#!/usr/bin/env python3
"""N real browser/CSS consumer probe for the row renderer, not a gameplay rig.
Historical 8b input foreground and dcba overflow reproduce red; current source
must keep both readable and contained at actual desktop/mobile drawer widths.
Does not mutate production source or inject application knowledge.
"""
from pathlib import Path
import subprocess,json
from playwright.sync_api import sync_playwright
ROOT=Path(__file__).resolve().parents[1]
def source(commit=None):
 if commit:return subprocess.check_output(['git','-C',str(ROOT),'show',commit+':web/static/js/megaron/ui/drawers/economy.js'],text=True)
 return (ROOT/'web/static/js/megaron/ui/drawers/economy.js').read_text()
def render(text):
 # Execute the actual pure renderer and actual escaping helper; no layout copy.
 fn=text[text.index('function standingGoodRowHTML'):text.index('function bindStandingGoodsRows')]
 code="import {esc} from './web/static/js/megaron/ui/format.js';\n"+fn+"\nconsole.log(JSON.stringify(standingGoodRowHTML([{key:'grain',name:'Grain'},{key:'silver',name:'Silver'}],'out')));"
 return json.loads(subprocess.check_output(['node','--input-type=module','-e',code],cwd=ROOT,text=True))
proof=[]
with sync_playwright() as p:
 browser=p.chromium.launch(ignore_default_args=['--disable-dev-shm-usage'])
 page=browser.new_page()
 for version in ['8b2b8315','dcba84cb','current']:
  html=render(source(None if version=='current' else version))
  for width in (362,390):
   page.set_content('<div id="fixture" style="width:'+str(width)+'px">'+html+'</div>')
   page.add_style_tag(path=str(ROOT/'web/static/megaron.css'))
   metric=page.locator('#fixture').evaluate("""e=>{let input=e.querySelector('input'),c=getComputedStyle(input),probe=document.createElement('span');probe.style.color=getComputedStyle(document.documentElement).getPropertyValue('--text');e.append(probe);let expected=getComputedStyle(probe).color;probe.remove();return {width:e.clientWidth,scrollWidth:e.scrollWidth,color:c.color,expected};}""")
   metric.update(version=version);proof.append(metric)
   if version=='8b2b8315':assert metric['color']!=metric['expected'],'must reproduce pale amount'
   elif version=='dcba84cb':assert metric['scrollWidth']>metric['width'],'must reproduce overflow'
   else:
    assert metric['color']==metric['expected'],metric
    assert metric['scrollWidth']<=metric['width'],metric
 browser.close()
print(json.dumps(proof,indent=2))
