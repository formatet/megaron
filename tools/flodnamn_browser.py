#!/usr/bin/env python3
"""Real map shell/renderer/input with scripted HTTP and saved real FOW response.
Only main's start IIFE is replaced; button/controller code stays intact.
"""
import functools,http.server,json,threading,sys,subprocess
from pathlib import Path
from playwright.sync_api import sync_playwright
ROOT=Path(__file__).resolve().parents[1];OUT=ROOT/'docs/reviews/flodnamn';OUT.mkdir(parents=True,exist_ok=True)
payload=json.loads((OUT/'map-fow.json').read_text());world=payload['world_id']
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
 for engine in ['firefox','chromium','webkit']:
  browser=getattr(pw,engine).launch()
  for width,height in [(1280,900),(390,844)]:
   page=browser.new_page(viewport=dict(width=width,height=height),device_scale_factor=1)
   errors=[];page.on('pageerror',lambda e:errors.append(str(e)));page.on('console',lambda m:errors.append(m.text) if m.type=='error' else None)
   page.route('https://**',lambda route:route.abort())
   if baseline:
    old=subprocess.check_output(['git','-C',str(ROOT),'show','f57eb4b0:web/static/js/megaron/render/map.js']).decode()
    page.route('**/static/js/megaron/render/map.js',lambda route:route.fulfill(content_type='text/javascript',body=old))
   page.route('**/static/js/megaron/main.js',lambda route:route.fulfill(content_type='text/javascript',body=main))
   def api(route):
    tail=route.request.url.split('/api/v1/')[-1].removeprefix('worlds/'+world+'/')
    if tail=='map':data=tiles
    elif tail=='provinces':data=provinces
    elif tail=='units':data=dict(units=[])
    elif tail=='storms':data=dict(storms=[])
    else:data=[]
    route.fulfill(content_type='application/json',body=json.dumps(data))
   page.route('**/api/v1/**',api)
   page.goto(base+'/static/map.html',wait_until='load')
   page.evaluate('''async({world})=>{
    localStorage.setItem('poleia_token','river-proof');window.requestAnimationFrame=()=>0;window.setInterval=()=>0;
    window.State=(await import('/static/js/megaron/state.js')).State;
    Object.assign(State,{WORLD_ID:world,MY_SETTLEMENT_ID:'mycenae-city',MY_PLAYER_ID:'Agamemnon',WORLD_STATE:'active',CURRENT_TICK:65,TICK_SECONDS:3600});
    window.map=await import('/static/js/megaron/render/map.js');map.initMap();await map.loadMap();
    State.camera={x:40,y:150,zoom:1};
    window.drawn=[];
    const ctx=map.canvas.getContext('2d'),original=ctx.fillText.bind(ctx);
    ctx.fillText=(text,x,y,...rest)=>{const m=ctx.getTransform();drawn.push({text,left:m.a*(x-ctx.measureText(text).width/2-1)+m.e,right:m.a*(x+ctx.measureText(text).width/2+1)+m.e,top:m.d*(y-1)+m.f,bottom:m.d*(y+9)+m.f});return original(text,x,y,...rest)};
    window.draw=()=>{drawn.length=0;State.animFrame=99;State.lastSeaTick=-1;State.dirty=true;map.render()};
    for(let i=0;i<12;i++)draw();
   }''',dict(world=world))
   row=page.evaluate('''baseline=>{
    const names=new Set(State.tileData.filter(t=>t.river_name&&t.tier!=='fog').map(t=>t.river_name));
    const rivers=drawn.filter(r=>names.has(r.text));const cities=drawn.filter(r=>State.provinceData.some(p=>p.name===r.text));
    const overlap=(a,b)=>a.left<b.right&&b.left<a.right&&a.top<b.bottom&&b.top<a.bottom;
    if(rivers.length!==(baseline?0:1))throw Error(`expected one label, got ${rivers.length}`);
    if(rivers.some(r=>cities.some(c=>overlap(r,c))))throw Error('river label overlaps city name');
    return {rivers,cities};
   }''',baseline)
   row.update(engine=engine,width=width);results.append(row)
   page.screenshot(path=str(OUT/f'{engine}-{width}-map{suffix}.png'))
   for q,form in [(0,'River'),(3,'Ford of'),(16,'Delta of')]:
    point=page.evaluate('''q=>{const p=map.hexPx(q,0);State.camera={x:90-p.x*2,y:220-p.y*2,zoom:1};for(let i=0;i<12;i++)draw();return {x:90,y:220,name:State.tileData.find(t=>t.q===q&&t.r===0).river_name}}''',q)
    rect=page.locator('#hex-canvas').bounding_box();page.mouse.click(rect['x']+point['x'],rect['y']+point['y'])
    text=page.locator('#ip-name').inner_text();assert baseline or text==f'{form} {point["name"]} ({q},0)',text
    page.screenshot(path=str(OUT/f'{engine}-{width}-{q}-panel{suffix}.png'))
    page.evaluate('map.closeInspect()')
   assert not errors,errors
   page.close()
  browser.close()
server.shutdown();(OUT/f'browser{suffix}.json').write_text(json.dumps(results,indent=2)+'\n');print('six engine/viewport combinations PASS, real names/panels/label boxes')
