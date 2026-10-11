#!/usr/bin/env python3
"""Real Firefox WS upgrade, heartbeat/pong survival and clean close after >60s.
Run against acceptance.sh's isolated localhost:8097 rig. No token or state writes.
"""
import datetime,json,re,subprocess,urllib.request
from pathlib import Path
from playwright.sync_api import sync_playwright
ROOT=Path(__file__).resolve().parents[1]
OUT=ROOT/'docs/reviews/ws-timeout'
BASE='http://localhost:8097'
health=json.load(urllib.request.urlopen(BASE+'/healthz'))
world=subprocess.check_output([str(ROOT/'tools/acceptance.sh'),'world'],text=True).strip()
started=datetime.datetime.now(datetime.timezone.utc).isoformat()
with sync_playwright() as pw:
 browser=pw.firefox.launch()
 page=browser.new_page()
 page.goto(BASE+'/healthz')
 # Firefox automatically answers protocol pings; the real hub also sends Heartbeat.
 page.evaluate('''world=>{window.probe={messages:[],errors:0}; window.sock=new WebSocket('ws://localhost:8097/ws/'+world);
 sock.onopen=()=>{probe.open=performance.now()};
 sock.onmessage=e=>{probe.messages.push({after_ms:performance.now()-probe.open,kind:JSON.parse(e.data).kind})};
 sock.onerror=()=>{probe.errors++};
 sock.onclose=e=>{probe.closed={code:e.code,clean:e.wasClean,after_ms:performance.now()-probe.open}};}''',world)
 page.wait_for_function('probe.open !== undefined',timeout=10000)
 page.wait_for_timeout(65000)
 assert page.evaluate('sock.readyState')==1,'WS died before 65s'
 page.evaluate('sock.close(1000,"timeout-proof")')
 page.wait_for_function('probe.closed !== undefined',timeout=10000)
 result=page.evaluate('probe')
 result.update(health=health,world=world,browser=browser.version,started=started)
 browser.close()
assert result['closed']['after_ms']>60000,result
assert result['closed']['code']==1000 and result['closed']['clean'] and result['errors']==0,result
assert sum(m['kind']=='Heartbeat' for m in result['messages'])>=2,result
# Give Logger/Timeout a chance to unwind after the read pump exits.
import time
time.sleep(1)
r=subprocess.run(['docker','logs','--since',started,'megaron-acc-server-1'],capture_output=True,text=True,check=True)
log=r.stdout+r.stderr
(OUT/'ws-server.log').write_text(log)
assert 'hijacked connection' not in log and not re.search(r' - 504 \d+B',log),log
assert '/ws/'+world in log,'missing actual WS request log'
(OUT/'ws-rigg.json').write_text(json.dumps(result,indent=2)+'\n')
print(json.dumps({'health':health,'browser':result['browser'],'closed':result['closed'],'heartbeats':sum(m['kind']=='Heartbeat' for m in result['messages']),'log':'no 504 or hijacked connection'}))
