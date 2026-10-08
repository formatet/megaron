#!/usr/bin/env python3
"""Controlled HTTP delays, actual production handlers, real Chromium DOM.
Usage: late_dom_probe.py OUT [CASE ...]. Nonzero if any named preservation fails.
These are isolated HTTP fixtures, explicitly not a real-game/FOW proof.
"""
import functools,http.server,json,pathlib,sys,threading
from playwright.sync_api import sync_playwright
ROOT=pathlib.Path(__file__).resolve().parents[1]
class Quiet(http.server.SimpleHTTPRequestHandler):
 def log_message(self,*args):pass
server=http.server.ThreadingHTTPServer(('127.0.0.1',0),functools.partial(Quiet,directory=str(ROOT)))
threading.Thread(target=server.serve_forever,daemon=True).start()
out=pathlib.Path(sys.argv[1]);out.mkdir(parents=True,exist_ok=True)
cases=sys.argv[2:] or ['garrison','recruit','transfer','build-hex','build-stale','build-refresh','grid','correspondence','automation','city-reload','war-reload','cult-reload','search']
results=[]
try:
 with sync_playwright() as p:
  browser=p.chromium.launch(headless=True,ignore_default_args=['--disable-dev-shm-usage'])
  for name in cases:
   page=browser.new_page(viewport={'width':1280,'height':900});page.goto(f'http://127.0.0.1:{server.server_port}/docs/reviews/late-dom-input-loss/contract.md')
   try:result=page.evaluate("async name => (await import('/tools/late_dom_probe.mjs')).run(name)",name)
   except Exception as e:result={'name':name,'passed':False,'error':str(e)}
   results.append(result);print(json.dumps(result),flush=True);page.screenshot(path=str(out/(name+'.png')));page.close()
  browser.close()
finally:server.shutdown();server.server_close()
(out/'results.json').write_text(json.dumps(results,indent=2)+'\n')
raise SystemExit(any(not x['passed'] for x in results))
