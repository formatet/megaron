#!/usr/bin/env python3
"""Real map shell/renderer/input with scripted HTTP and saved real FOW response.
Only main's start IIFE is replaced; button/controller code stays intact.
"""
import functools,http.server,json,threading,sys,subprocess
from pathlib import Path
from playwright.sync_api import sync_playwright
ROOT=Path(__file__).resolve().parents[1];OUT=ROOT/'docs/reviews/flod-terrangrad';OUT.mkdir(parents=True,exist_ok=True)
payload=json.loads((ROOT/'docs/reviews/flodnamn/map-fow.json').read_text());world=payload['world_id']
# Context is explicitly synthetic land; named river tiles are the actual handler response.
tiles=payload['tiles']+[dict(q=q,r=r,terrain='plains',visible=True,tier='live') for q in range(-3,20) for r in range(-5,6) if r!=0 or q<0 or q>18]
provinces=[dict(q=3,r=-2,name='Mycenae',owner='Agamemnon',own=True,is_capital=True,id='mycenae',settlement_id='mycenae-city',walls=1),dict(q=8,r=-3,name='Tiryns',owner='Nestor',own=False,id='tiryns',settlement_id='tiryns-city',walls=1)]
class Handler(http.server.SimpleHTTPRequestHandler):
 def log_message(self,*args):pass
server=http.server.ThreadingHTTPServer(('127.0.0.1',0),functools.partial(Handler,directory=str(ROOT/'web')));threading.Thread(target=server.serve_forever,daemon=True).start();base=f'http://127.0.0.1:{server.server_port}'
main=(ROOT/'web/static/js/megaron/main.js').read_text().split('// ── Start ─')[0]
baseline='--baseline' in sys.argv
suffix='-before' if baseline else ''
results=[]
with sync_playwright() as pw:
 browser=pw.firefox.launch();page=browser.new_page(viewport=dict(width=1280,height=900),device_scale_factor=1)
 errors=[];page.on('pageerror',lambda e:errors.append(str(e)));page.on('console',lambda m:errors.append(m.text) if m.type=='error' else None)
 page.route('https://**',lambda route:route.abort())
 if baseline:
  old=subprocess.check_output(['git','-C',str(ROOT),'show','3e25da49:web/static/js/megaron/render/map.js']).decode()
  page.route('**/static/js/megaron/render/map.js',lambda route:route.fulfill(content_type='text/javascript',body=old))
 page.route('**/static/js/megaron/main.js',lambda route:route.fulfill(content_type='text/javascript',body=main))
 def api(route):
  tail=route.request.url.split('/api/v1/')[-1].removeprefix('worlds/'+world+'/')
  data=tiles if tail=='map' else provinces if tail=='provinces' else dict(units=[]) if tail=='units' else dict(storms=[]) if tail=='storms' else []
  route.fulfill(content_type='application/json',body=json.dumps(data))
 page.route('**/api/v1/**',api);page.goto(base+'/static/map.html',wait_until='load')
 page.evaluate("""async({world})=>{
 localStorage.setItem('poleia_token','river-proof');window.requestAnimationFrame=()=>0;window.setInterval=()=>0;
 window.State=(await import('/static/js/megaron/state.js')).State;
 Object.assign(State,{WORLD_ID:world,MY_SETTLEMENT_ID:'mycenae-city',MY_PLAYER_ID:'Agamemnon',WORLD_STATE:'active',CURRENT_TICK:65,TICK_SECONDS:3600});
 window.map=await import('/static/js/megaron/render/map.js');map.initMap();await map.loadMap();
 window.draw=()=>{State.animFrame=99;State.lastSeaTick=-1;State.dirty=true;map.render()};
 }""",dict(world=world))
 for q,form,label in [(0,'River','River'),(3,'Ford of','River ford'),(16,'Delta of','River delta')]:
  for rural in [False,True]:
   name=page.evaluate("""async({q,rural})=>{
    const p=map.hexPx(q,0);State.camera={x:160-p.x*2,y:260-p.y*2,zoom:1};State.ruralData=rural?[{q,r:0,building_type:'farm',name:'Mycenae',province_id:'mycenae'}]:[];
    for(let n=0;n<1500;n++){draw();if(map.markrasterStats.pending===0)break;await new Promise(r=>setTimeout(r,16));}draw();
    return State.tileData.find(t=>t.q===q&&t.r===0).river_name;
   }""",dict(q=q,rural=rural))
   rect=page.locator('#hex-canvas').bounding_box();page.mouse.click(rect['x']+160,rect['y']+260)
   title=page.locator('#ip-name').inner_text();terrain=page.locator('#ip-terrain').inner_text()
   assert title==f'{form} {name} ({q},0)',title
   assert terrain==(f'{form} {name}' if baseline else label),(title,terrain,label)
   results.append(dict(q=q,rural=rural,title=title,terrain=terrain));page.screenshot(path=str(OUT/f'firefox-1280-{q}-{"rural" if rural else "hex"}{suffix}.png'));page.evaluate('map.closeInspect()')
 assert not errors,errors;browser.close()
server.shutdown();(OUT/f'browser{suffix}.json').write_text(json.dumps(results,indent=2)+'\n');print('Firefox1280: three river forms, hex+rural PASS')
