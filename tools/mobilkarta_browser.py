#!/usr/bin/env python3
"""Production map/CSS/menu, explicit read-only HTTP; no live game claims.
Real touch taps and desktop mouse in every engine; synthetic pointer gestures
in every engine; Chromium CDP native gestures additionally exercise UA scroll.
"""
import argparse,datetime,functools,http.server,json,re,threading
from pathlib import Path
from playwright.sync_api import sync_playwright,expect
ROOT=Path(__file__).resolve().parents[1]
p=argparse.ArgumentParser(description=__doc__);p.add_argument('--source-root',type=Path,default=ROOT);p.add_argument('--baseline',action='store_true');p.add_argument('--output',type=Path,default=ROOT/'docs/reviews/mobilkarta/browser');p.add_argument('--browsers',nargs='+',default=['firefox','chromium','webkit']);p.add_argument('--touch-mutation',action='store_true');a=p.parse_args();ROOT=a.source_root.resolve();a.output.mkdir(parents=True,exist_ok=True)
W='11111111-1111-1111-1111-111111111111';S='22222222-2222-2222-2222-222222222222'
provinces=[dict(id='mycenae',settlement_id=S,name='Mycenae',owner='Agamemnon',own=True,is_capital=True,q=0,r=0,walls=1),dict(id='tiryns',settlement_id='tiryns-city',name='Tiryns',owner='Nestor',own=False,q=3,r=0,walls=1)]
units=[dict(id='bronze-guard',name='Bronze Guard',display_name='Bronze Guard',type='spearman',category='land',status='garrison',settlement_id=S,home_name='Mycenae',deployable=True,size=100,max_size=100,stance='sentry')]
tiles=[dict(q=q,r=r,terrain='plains',visibility='live',deposits=[]) for q in range(-15,16) for r in range(-15,16)]
class Handler(http.server.SimpleHTTPRequestHandler):
 def log_message(self,*args):pass
server=http.server.ThreadingHTTPServer(('127.0.0.1',0),functools.partial(Handler,directory=str(ROOT/'web')));threading.Thread(target=server.serve_forever,daemon=True).start();base=f'http://127.0.0.1:{server.server_port}'
html=re.sub(r'<script\b[^>]*>.*?</script>','',(ROOT/'web/static/map.html').read_text(),flags=re.S)
records=[]
def pointer(page,kind,x,y,pid=1,pointer_type='touch'):
 page.evaluate('''([kind,x,y,id,pointerType])=>document.getElementById('hex-canvas').dispatchEvent(new PointerEvent(kind,{pointerId:id,pointerType,isPrimary:id===1,clientX:x,clientY:y,buttons:kind==='pointerup'?0:1,bubbles:true}))''',[kind,x,y,pid,pointer_type])
def camera(page):return page.evaluate('({...State.camera})')
def reset(page):
 page.evaluate('''()=>{map.closeInspect();closeMarchCtx();State.selectedHex=null;State.camera={x:150,y:250,zoom:1};State.dirty=true}''');page.wait_for_timeout(80)
def native_drag(page,session):
 for kind,coords in [('touchStart',[(150,400)]),* [('touchMove',[(150,y)]) for y in [390,370,340,300,260]],('touchEnd',[])]:
  session.send('Input.dispatchTouchEvent',dict(type=kind,touchPoints=[dict(x=x,y=y,id=i) for i,(x,y) in enumerate(coords)]));page.wait_for_timeout(40)
try:
 with sync_playwright() as pw:
  for engine in a.browsers:
   browser=getattr(pw,engine).launch()
   for mode,width,height in [('desktop',1280,900),('mobile',390,844)]:
    if a.touch_mutation and mode!='mobile':continue
    page=browser.new_page(viewport=dict(width=width,height=height),has_touch=mode=='mobile',is_mobile=engine!='firefox' and mode=='mobile',locale='en-GB',timezone_id='Europe/Stockholm');page.clock.set_fixed_time(datetime.datetime(2026,10,9,7,0,tzinfo=datetime.timezone.utc));errors=[];unexpected=[];calls=[]
    page.on('pageerror',lambda e:errors.append(str(e)))
    page.route('https://**',lambda r:r.abort())
    page.route('**/fixture',lambda r:r.fulfill(content_type='text/html',body=html))
    def api(route):
     req=route.request;tail=req.url.split('/api/v1/')[-1].removeprefix('worlds/'+W+'/');calls.append(tail)
     assert req.method=='GET',req.method
     assert req.headers.get('authorization')=='Bearer mobile-read-fixture'
     if tail=='map':data=tiles
     elif tail=='provinces':data=provinces
     elif tail=='units':data=dict(units=units)
     elif tail in ['marches','messengers','trades','rural-projections','foreign-units','settlements/placement-roster']:data=[]
     elif tail=='provinces/tiryns/army':data=dict(Spearman=1)
     elif tail.startswith('units/bronze-guard/march-preview?'):data=dict(available=True,arrival_tick=68,duration_ticks=3,arrives_at_utc='2026-10-09T10:00:00Z')
     else:unexpected.append(tail);route.fulfill(status=500,body='{}');return
     route.fulfill(content_type='application/json',body=json.dumps(data))
    page.route('**/api/v1/**',api);page.goto(base+'/fixture',wait_until='load')
    page.evaluate('''async([world,s,provinces])=>{
     localStorage.setItem('poleia_token','mobile-read-fixture');window.MusicPlayer={update(){}};
     window.State=(await import('/static/js/megaron/state.js')).State;
     Object.assign(State,{WORLD_ID:world,MY_SETTLEMENT_ID:s,MY_PLAYER_ID:'Agamemnon',CURRENT_TICK:65,TICK_SECONDS:3600,TICK_ANCHOR_MS:Date.now(),WORLD_STATE:'active',provinceData:provinces});
     document.getElementById('gt-wanax').textContent='Wanax Agamemnon';document.getElementById('net-status').textContent='Mycenae · Tiryns';document.getElementById('cel-date').textContent='the Olive · Year 1';
     window.map=await import('/static/js/megaron/render/map.js');const march=await import('/static/js/megaron/ui/marchctx.js');window.openMarchCtx=march.openMarchCtx;window.closeMarchCtx=march.closeMarchCtx;
     window.warFocusUnit=()=>{};map.initMap();
    }''',[W,S,provinces]);page.wait_for_function('State.tileData.length>0');reset(page)
    rect=page.locator('#hex-canvas').bounding_box();y=rect['y']+350
    before=camera(page);pointer(page,'pointerdown',150,y);pointer(page,'pointermove',190,y+60);pointer(page,'pointerup',190,y+60);after=camera(page)
    nav=page.locator('.win-trigger').evaluate_all('''els=>els.map(el=>{const range=document.createRange();const text=[...el.childNodes].find(n=>n.nodeType===3&&n.textContent.trim());range.selectNode(text);const t=range.getBoundingClientRect(),b=el.getBoundingClientRect();return {name:text.textContent.trim(),fits:t.left>=b.left+4&&t.right<=b.right-4,text:[t.left,t.right],button:[b.left,b.right]}})''')
    native=None
    if engine=='chromium':
     reset(page);session=page.context.new_cdp_session(page)
     # An explicit scrollable host probes UA ownership of a drag. The game
     # normally has no page scroll; without touch-action the UA cancels its gesture.
     page.evaluate('''()=>{window.nativeEvents=[];for(const type of ['pointerdown','pointermove','pointerup','pointercancel'])document.getElementById('hex-canvas').addEventListener(type,e=>nativeEvents.push({type,trusted:e.isTrusted,pointerType:e.pointerType}));window.bodyStyle=document.body.getAttribute('style');document.body.style.overflow='auto';document.body.style.height='auto';const spacer=document.createElement('div');spacer.id='scroll-probe';spacer.style.height='1000px';document.body.append(spacer)}''')
     nb=camera(page);native_drag(page,session);native=dict(before=nb,after=camera(page),scrollY=page.evaluate('scrollY'),events=page.evaluate('nativeEvents'))
     if a.touch_mutation:
      records.append(dict(engine=engine,mode=mode,native=native));print('U native touch drag keeps page still and moves camera: '+json.dumps(native),flush=True)
      assert native['scrollY']==0 and native['after']['y']<native['before']['y']-50,'U native touch drag keeps page still and moves camera'
      page.close();continue
     if not a.baseline:
      assert native['scrollY']==0 and native['after']['y']<native['before']['y']-50,('U native touch drag keeps page still and moves camera',native)
     page.evaluate('''()=>{document.getElementById('scroll-probe').remove();if(bodyStyle===null)document.body.removeAttribute('style');else document.body.setAttribute('style',bodyStyle);scrollTo(0,0)}''')
     if not a.baseline:
      # Native two-finger pinch and 550ms hold supplement the cross-engine
      # synthetic sequence probes. Target empty land so no game writes occur.
      reset(page);pz=camera(page)['zoom']
      for kind,coords in [('touchStart',[(100,350),(200,350)]),('touchMove',[(80,350),(220,350)]),('touchMove',[(60,350),(240,350)]),('touchEnd',[])]:
       session.send('Input.dispatchTouchEvent',dict(type=kind,touchPoints=[dict(x=x,y=y,id=i) for i,(x,y) in enumerate(coords)]));page.wait_for_timeout(60)
      native['pinch_zoom']=[pz,camera(page)['zoom']];assert camera(page)['zoom']>pz*1.5,native
      reset(page);session.send('Input.dispatchTouchEvent',dict(type='touchStart',touchPoints=[dict(x=200,y=350,id=0)]));page.wait_for_timeout(550);expect(page.locator('#march-ctx')).to_be_visible();session.send('Input.dispatchTouchEvent',dict(type='touchEnd',touchPoints=[]));native['long_press']=True;assert page.locator('#inspect-panel').is_hidden();page.evaluate('closeMarchCtx()')
    if a.baseline:
     assert after==before,(before,after)
     if native:assert native['before']==native['after'],native
     if mode=='mobile':assert not all(n['fits'] for n in nav),nav
     page.screenshot(path=str(a.output/f'{engine}-{mode}-baseline.png'));records.append(dict(engine=engine,mode=mode,drag_before=before,drag_after=after,native=native,nav=nav));page.close();continue
    assert abs(after['x']-before['x']-40)<.1 and abs(after['y']-before['y']-60)<.1,(before,after)
    assert page.evaluate('State.selectedHex===null'),'drag accidentally selects hex'
    assert all(n['fits'] for n in nav),nav
    assert page.locator('#hex-canvas').evaluate("e=>getComputedStyle(e).touchAction")=='none'
    assert page.locator('.drawer-body').first.evaluate("e=>getComputedStyle(e).touchAction")=='auto'
    if native:assert native['after']['y']<native['before']['y']-50,native
    reset(page);page.screenshot(path=str(a.output/f'{engine}-{mode}-overview.png'));pinch_before=camera(page)
    pointer(page,'pointerdown',100,y,1);pointer(page,'pointerdown',200,y,2);pointer(page,'pointermove',70,y,1);pointer(page,'pointermove',230,y,2);pointer(page,'pointerup',70,y,1);pointer(page,'pointerup',230,y,2)
    pinch_after=camera(page);assert abs(pinch_after['zoom']-1.6)<.01,pinch_after
    # The world point under the initial centroid remains under its final centroid.
    local_y=y-rect['y'];assert abs((local_y-pinch_before['y'])/pinch_before['zoom']-(local_y-pinch_after['y'])/pinch_after['zoom'])<.1
    assert page.evaluate('State.selectedHex===null')
    page.screenshot(path=str(a.output/f'{engine}-{mode}-map.png'))
    reset(page)
    # Centre Tiryns independently of viewport; hexPx is the real renderer transform.
    xy=page.evaluate('''()=>{const p=map.hexPx(3,0),r=document.getElementById('hex-canvas').getBoundingClientRect();State.camera.x=200-p.x*map.SCALE;State.camera.y=250-p.y*map.SCALE;State.dirty=true;return [r.left+200,r.top+250]}''');page.wait_for_timeout(80)
    (page.touchscreen.tap(*xy) if mode=='mobile' else page.mouse.click(*xy));expect(page.locator('#inspect-panel')).to_be_visible();expect(page.locator('#ip-name')).to_have_text('Tiryns');expect(page.locator('#ip-owner')).to_contain_text('Nestor')
    page.screenshot(path=str(a.output/f'{engine}-{mode}-tap.png'));page.evaluate('map.closeInspect()')
    pointer(page,'pointerdown',*xy);page.wait_for_timeout(550);expect(page.locator('#march-ctx')).to_be_visible();expect(page.locator('#mctx-name')).to_have_text('Tiryns');pointer(page,'pointerup',*xy)
    expect(page.locator('#inspect-panel')).not_to_be_visible();expect(page.locator('#mctx-units')).to_contain_text('Bronze Guard');expect(page.locator('#mctx-eta')).to_contain_text('Estimated arrival');page.screenshot(path=str(a.output/f'{engine}-{mode}-orders.png'));page.evaluate('closeMarchCtx()')
    mouse_emulated=engine=='firefox' and mode=='mobile'
    if mouse_emulated:
     # Firefox Browser.setTouchOverride suppresses native mouse Pointer Events.
     # Native mouse is proved in its separate desktop context; use pointer probes here.
     pointer(page,'pointermove',*xy,pointer_type='mouse');expect(page.locator('#tile-tooltip')).to_contain_text('Tiryns')
     pointer(page,'pointerdown',*xy,pointer_type='mouse');pointer(page,'pointerup',*xy,pointer_type='mouse')
    else:
     page.mouse.move(xy[0]-15,xy[1]-15);page.mouse.move(*xy);expect(page.locator('#tile-tooltip')).to_contain_text('Tiryns');page.mouse.click(*xy)
    expect(page.locator('#ip-name')).to_have_text('Tiryns');page.evaluate('map.closeInspect()')
    if mouse_emulated:pointer(page,'contextmenu',*xy,pointer_type='mouse')
    else:page.mouse.click(*xy,button='right')
    expect(page.locator('#march-ctx')).to_be_visible();expect(page.locator('#mctx-name')).to_have_text('Tiryns');page.evaluate('closeMarchCtx()')
    mb=camera(page)
    if mouse_emulated:
     pointer(page,'pointerdown',150,y,pointer_type='mouse');pointer(page,'pointermove',180,y+50,pointer_type='mouse');pointer(page,'pointerup',180,y+50,pointer_type='mouse')
    else:page.mouse.move(150,y);page.mouse.down();page.mouse.move(180,y+50,steps=4);page.mouse.up()
    ma=camera(page);assert ma['x']>mb['x']+25 and ma['y']>mb['y']+45,(mb,ma)
    page.mouse.move(150,y);zb=camera(page)['zoom']
    if engine=='webkit' and mode=='mobile':page.locator('#hex-canvas').dispatch_event('wheel',dict(clientX=150,clientY=y,deltaY=-100))
    else:page.mouse.wheel(0,-100)
    page.wait_for_timeout(70);assert camera(page)['zoom']>zb
    kb=camera(page);page.keyboard.down('ArrowRight');page.wait_for_timeout(100);page.keyboard.up('ArrowRight');assert camera(page)['x']<kb['x']
    page.locator('#hex-canvas').dispatch_event('pointerleave')
    page.evaluate("async()=>{window.cx=await import('/static/js/megaron/ui/codex.js');cx.initCodex();await cx.openCodex('screen')}")
    expect(page.locator('#codex-body')).to_contain_text('On a touchscreen');page.wait_for_timeout(220)
    # CSS offsets must leave the whole bottom menu visible alongside either panel.
    nav_top=page.locator('.game-nav').bounding_box()['y'];cb=page.locator('#codex-panel').bounding_box();assert cb['x']>=-1 and cb['x']+cb['width']<=width+1,cb;assert cb['y']+cb['height']<=nav_top+1,cb
    codex_scroll=None
    if engine=='chromium' and mode=='mobile':
     body=page.locator('#codex-body');assert body.evaluate('e=>e.scrollHeight>e.clientHeight')
     body.evaluate('e=>e.scrollTop=0');native_drag(page,session);page.wait_for_timeout(100);codex_scroll=body.evaluate('e=>e.scrollTop');assert codex_scroll>50,('Codex native touch scroll',codex_scroll)
    page.locator('#codex-body').evaluate("e=>e.scrollTop=e.scrollHeight");page.screenshot(path=str(a.output/f'{engine}-{mode}-controls.png'))
    expect(page.locator('#codex-body')).to_contain_text('hold still briefly to open orders')
    page.evaluate('cx.closeCodex()')
    assert not errors and not unexpected,(errors,unexpected)
    records.append(dict(engine=engine,mode=mode,drag_before=before,drag_after=after,pinch_before=pinch_before,pinch_after=pinch_after,native=native,nav=nav,native_tap=mode=='mobile',synthetic_long_press=True,mouse_parity='synthetic' if mouse_emulated else 'native',errors=errors,codex_scroll=codex_scroll,requests=calls));print(engine+' '+mode+': PASS',flush=True);page.close()
   browser.close()
finally:
 server.shutdown();(a.output/'proof.json').write_text(json.dumps(records,indent=2)+'\n')
