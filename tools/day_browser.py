#!/usr/bin/env python3
"""Real CSS/controllers, scripted HTTP. Firefox first; no live-game claims.
Names are human fixtures; all routes are explicitly listed, writes only mark
fixture notifications read. Both calendar alternatives keep today's default.
"""
from pathlib import Path
import functools,http.server,json,re,threading
from playwright.sync_api import sync_playwright,expect
ROOT=Path(__file__).resolve().parents[1];OUT=ROOT/'docs/reviews/day/browser';OUT.mkdir(parents=True,exist_ok=True)
WORLD='11111111-1111-1111-1111-111111111111';P1='22222222-2222-2222-2222-222222222222';P2='33333333-3333-3333-3333-333333333333';S1='44444444-4444-4444-4444-444444444444';S2='55555555-5555-5555-5555-555555555555'
provinces=[dict(id=P1,settlement_id=S1,name='Mycenae',owner='Agamemnon',own=True,is_capital=True,q=0,r=0,culture='achaean',walls=1),dict(id=P2,settlement_id=S2,name='Tiryns',owner='Agamemnon',own=True,is_capital=False,q=4,r=0,culture='achaean',walls=1)]
overview=[dict(id=S1,population=1000,grain_prod_rate=140,grain_consum_rate=100,sitos=dict(coverage_ticks=40,low_ticks=10,high_ticks=30,granary_total=500,granary_cap=1000,food_net_per_tick=40)),dict(id=S2,population=800,grain_prod_rate=40,grain_consum_rate=80,sitos=dict(coverage_ticks=1,low_ticks=10,high_ticks=30,granary_total=0,food_net_per_tick=-40))]
goods=[dict(key='silver',name='Silver',amount=1200,rate_per_tick=4,cap=3000),dict(key='grain',name='Grain',amount=4000,rate_per_tick=40,cap=10000,producible=True),dict(key='bronze',name='Bronze',amount=100,rate_per_tick=1,cap=1000,producible=True)]
units=[dict(id='66666666-6666-6666-6666-666666666666',name='Bronze Guard',type='spearman',category='land',size=100,max_size=100,status='positioned',q=2,r=0,settlement_id=S1,home_name='Mycenae',provision_days=1,stance='sentry',expedition=dict(area_q=3,area_r=1,length_ticks=12,turn_tick=71,home_by_tick=77,homeward=False)),dict(id='77777777-7777-7777-7777-777777777777',name='Sacred Dolphin',type='galley',category='naval',size=1,status='positioned',q=2,r=1,settlement_id=S1,home_name='Mycenae',provision_days=1,hull=5,hull_max=5,march_intent='pickup_wait',waiting_until_tick=68,pickup_for='Bronze Guard')]
fp=dict(active=True,q=0,r=0,population=1000,spearmen_in_field=2,grain=dict(amount=100,ticks_left=1),silver=dict(amount=500,ticks_left=3),tick_seconds=3600)
forecast=dict(grain=dict(base_per_tick=120,est_net_per_tick=20,consumption_per_tick=100,seed=100,ticks_until_empty=1),goods=dict(timber=20,stone=4),catchment=[dict(known=True,terrain='plains',q=q,r=r) for q in range(-2,3) for r in range(-2,3) if abs(q+r)<=2],unknown_hexes=0,food_self_sufficient=True)
notifications=[dict(id='event-'+str(i),kind=k,level=3,created_at='2026-10-09T05:00:00Z',body=b,read_at=None) for i,(k,b) in enumerate([
 ('ForeignMarchSightedV2',dict(owner='Nestor',unit_type='spearman',size=100,threatens_name='Tiryns',eta_if_tick=68)),
 ('SubsistenceWarning',dict(name='Tiryns',net_per_tick=-40,ticks_left=1,tier='warning')),
 ('CityOccupied',dict(name='Tiryns',role='attacker',occupation_ticks_to_annex=3)),
 ('ExpeditionTurnedHome',dict(name='Bronze Guard',area_q=3,area_r=1,reason='half_time',arrive_tick=68)),
])]
tiles=[dict(q=q,r=r,terrain='plains',visibility='live',deposits=[],food=100) for q in range(-10,11) for r in range(-8,9)]
class Handler(http.server.SimpleHTTPRequestHandler):
 def log_message(self,*args):pass
server=http.server.ThreadingHTTPServer(('127.0.0.1',0),functools.partial(Handler,directory=str(ROOT/'web')));threading.Thread(target=server.serve_forever,daemon=True).start();base=f'http://127.0.0.1:{server.server_port}'
html=re.sub(r'<script\b[^>]*>.*?</script>','',(ROOT/'web/static/map.html').read_text(),flags=re.S)
results=[]
try:
 with sync_playwright() as pw:
  for engine in ['firefox','chromium','webkit']:
   browser=getattr(pw,engine).launch()
   try:
    for mode,width,height in [('desktop',1280,900),('390',390,844)]:
     page=browser.new_page(viewport=dict(width=width,height=height),locale='en-GB');errors=[];requests=[];unexpected=[]
     page.on('pageerror',lambda e:errors.append(str(e)))
     page.on('console',lambda m:print(m.type+': '+m.text+' '+str([a.evaluate('(v)=>v?.stack||String(v)') for a in m.args]),flush=True) if m.type=='error' else None)
     page.route('https://**',lambda route:route.abort())
     page.route('**/fixture',lambda route:route.fulfill(content_type='text/html',body=html))
     def api(route):
      req=route.request;path=req.url.split('/api/v1/')[-1];tail=path.removeprefix('worlds/'+WORLD+'/');requests.append(dict(method=req.method,path=tail))
      data=None
      if req.method=='POST' and tail=='notifications/read-all':data={}
      elif req.method!='GET':unexpected.append(tail);route.fulfill(status=500,body='{}');return
      elif tail=='map':data=tiles
      elif tail=='provinces':data=provinces
      elif path=='units':data=[]
      elif tail=='units':data=dict(units=units)
      elif tail in ['marches','messengers','trades','rural-projections','foreign-units','settlements/placement-roster','gossip']:data=[]
      elif tail=='settlements/overview':data=overview
      elif tail in ['provinces/'+P1+'/goods','provinces/'+P2+'/goods']:data=goods
      elif tail in ['provinces/'+P1+'/actions','provinces/'+P2+'/actions']:data=[]
      elif tail=='provinces/'+P1:data=dict(settlement=dict(population=1000,army={},buildings=[],can_recruit=[],resources={}))
      elif tail=='retreat-default':data=dict(by_loyalty=True,hold_to_last_man=False,retreat_at_loss=0.5)
      elif tail.startswith('notifications?'):data=dict(notifications=[] if 'kind=' in tail else notifications)
      elif tail=='founding/status':data=fp
      elif tail.startswith('colonize-preview?'):data=forecast
      else:unexpected.append(tail);print('UNEXPECTED '+tail,flush=True);route.fulfill(status=500,body='{}');return
      route.fulfill(content_type='application/json',body=json.dumps(data))
     page.route('**/api/v1/**',api)
     page.goto(base+'/fixture',wait_until='domcontentloaded')
     page.evaluate('''async([world,s1,provinces])=>{
      localStorage.setItem('poleia_token','day-render-fixture');
      window.MusicPlayer={update(){}};window.renderColonizePreviewHTML=(await import('/static/js/megaron/ui/marchctx.js')).renderColonizePreviewHTML;
      const {State}=await import('/static/js/megaron/state.js');window.State=State;
      Object.assign(State,{WORLD_ID:world,MY_SETTLEMENT_ID:s1,MY_PLAYER_ID:'Agamemnon',CURRENT_TICK:65,TICK_SECONDS:3600,TICK_ANCHOR_MS:Date.now(),WORLD_STATE:'active',provinceData:provinces});
      document.getElementById('gt-wanax').textContent='Wanax Agamemnon';document.getElementById('net-status').textContent='Mycenae · Tiryns';document.getElementById('cel-date').textContent='the Olive · Year 1';
      window.economy=await import('/static/js/megaron/ui/drawers/economy.js');window.war=await import('/static/js/megaron/ui/drawers/war.js');window.notif=await import('/static/js/megaron/ui/drawers/notif.js');window.map=await import('/static/js/megaron/render/map.js');
      map.initMap();
     }''',[WORLD,S1,provinces])
     page.wait_for_function('State.tileData.length>0')
     def shot(label,selector,words):
      el=page.locator(selector);expect(el).to_be_visible();page.wait_for_timeout(250);text=el.inner_text()
      for word in words:assert word.casefold() in text.casefold(),(label,word,text)
      assert not re.search(r'NaN|undefined|Invalid Date',text),(label,text)
      assert not re.search(r'game[ -]day|\bticks?\b|/tick|≈\s*\d+\s*days',text,re.I),(label,text)
      assert not re.search(r'[0-9a-f]{8}-[0-9a-f]{4}-',text),(label,text)
      box=el.bounding_box();assert box['x']>=-1 and box['x']+box['width']<=width+1,(label,box)
      page.screenshot(path=str(OUT/f'{engine}-{mode}-{label}.png'))
      results.append(dict(engine=engine,viewport=mode,surface=label,text=text,bounds=box))
     page.evaluate("document.getElementById('drawer-economy').classList.add('open');economy.loadEconomyDrawer()")
     expect(page.locator('#ectab-goods')).to_contain_text('40 days')
     shot('economy','#drawer-economy',['Mycenae','Tiryns','40 days','1 day','each day'])
     page.evaluate("document.getElementById('drawer-economy').classList.remove('open');document.getElementById('drawer-war').classList.add('open');war.loadWarDrawer()")
     expect(page.locator('#wtab-army')).to_contain_text('Bronze Guard')
     shot('war','#drawer-war',['Bronze Guard','Sacred Dolphin','1 day','12 days','day 68'])
     page.evaluate("document.getElementById('drawer-war').classList.remove('open');document.getElementById('drawer-notif').classList.add('open');notif.loadNotifDrawer()")
     expect(page.locator('#notif-body')).to_contain_text('Nestor')
     shot('notifications','#drawer-notif',['Nestor','Tiryns','day 68','1 day','3 unchallenged days'])
     for show in [True,False]:
      page.evaluate('''show=>{document.querySelector('.notif-date-header').outerHTML=notif.notifDateHeader({day:1,month:1,monthName:'Pithoi',year:1},State,show)}''',show)
      shot('calendar-'+('with' if show else 'without'),'#drawer-notif',['Day 1 of Pithoi'+(' (1)' if show else '')+', Year 1'])
     page.evaluate('''(fp)=>{
      document.getElementById('drawer-notif').classList.remove('open');State.founderPhase=fp;State.provinceData=[];State.unitsData=[{type:'nomadic_host',name:'Agamemnon’s Host',q:0,r:0}];State.tileData=[{q:0,r:0,terrain:'plains',deposits:[]}];State.camera={x:100,y:100,zoom:1};
      const canvas=document.getElementById('hex-canvas'),rect=canvas.getBoundingClientRect();
      for(const type of ['mousedown','mouseup'])canvas.dispatchEvent(new MouseEvent(type,{clientX:rect.left+100,clientY:rect.top+100,bubbles:true}));
     }''',fp)
     expect(page.locator('#ip-host-details')).to_be_visible();page.locator('#ip-host-details').evaluate('(e)=>e.open=true')
     expect(page.locator('#ip-found-summary')).to_contain_text('Feeds itself')
     shot('host','#inspect-panel',['Food lasts 1 day','Escort pay lasts 3 days'])
     assert not errors and not unexpected,(errors,unexpected)
     results.append(dict(engine=engine,viewport=mode,requests=requests,errors=errors,unexpected=unexpected))
     page.close()
   finally:browser.close()
finally:server.shutdown()
(OUT/'proof.json').write_text(json.dumps(results,indent=2)+'\n')
print('PASS: Firefox, Chromium, WebKit × desktop/390; six pictures per viewport; named fixtures; scripted HTTP.')
