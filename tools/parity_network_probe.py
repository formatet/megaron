#!/usr/bin/env python3
"""Attribute startup console/resource errors without suppressing analytics."""
import json,pathlib
import browser_parity as b
from playwright.sync_api import sync_playwright
out=pathlib.Path('docs/reviews/paritet-20261011');results=[]
with sync_playwright() as pw:
 for engine in ['firefox','chromium','webkit']:
  browser=getattr(pw,engine).launch();ctx=browser.new_context(viewport={'width':390,'height':844});t=b.login('Eirene');ctx.add_cookies([{'name':'poleia_token','value':t,'url':b.HOST}]);ctx.add_init_script('localStorage.setItem("poleia_token",'+json.dumps(t)+')');page=ctx.new_page();row=dict(engine=engine,pageerrors=[],console_errors=[],failed_requests=[])
  page.on('pageerror',lambda e:row['pageerrors'].append(str(e)))
  page.on('console',lambda m:row['console_errors'].append(m.text) if m.type=='error' else None)
  page.on('requestfailed',lambda r:row['failed_requests'].append({'url':r.url,'failure':r.failure}))
  page.goto(b.HOST+'/play',wait_until='networkidle');page.wait_for_timeout(1000);results.append(row);browser.close()
(out/'network-probe.json').write_text(json.dumps(results,indent=2)+'\n');print(json.dumps(results,indent=2))
