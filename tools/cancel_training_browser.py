#!/usr/bin/env python3
"""Real drawer/controllers against isolated scripted HTTP, never live accounts."""
import functools,http.server,json,threading
from pathlib import Path
from playwright.sync_api import sync_playwright,expect
ROOT=Path(__file__).resolve().parents[1]
OUT=ROOT/'docs/reviews/cancel-training';OUT.mkdir(parents=True,exist_ok=True)
class Handler(http.server.SimpleHTTPRequestHandler):
 def log_message(self,*args):pass
server=http.server.ThreadingHTTPServer(('127.0.0.1',0),functools.partial(Handler,directory=str(ROOT/'web')))
threading.Thread(target=server.serve_forever,daemon=True).start()
base=f'http://127.0.0.1:{server.server_port}'
unit={'id':'spears','type':'spearman','category':'land','status':'training','size':100,'settlement_id':'city','name':'Third Spearmen','can_cancel_training':True,'build_complete_at':'2099-01-01T00:00:00Z','upkeep_context':'inactive','upkeep_food_per_tick':.5,'upkeep_silver_per_tick':0}
ship={**unit,'id':'ship','name':'Labyrinth Wind','type':'galley','category':'naval','status':'forming','size':1,'crew':20,'upkeep_food_per_tick':.04,'upkeep_silver_per_tick':1.5}
try:
 with sync_playwright() as pw:
  browser=pw.firefox.launch()
  for run in range(2):
   page=browser.new_page(viewport={'width':1100,'height':820},device_scale_factor=1)
   state={'units':[unit.copy(),ship.copy()],'posts':[]};errors=[]
   page.on('pageerror',lambda e:errors.append(str(e)))
   page.route('**/fixture',lambda r:r.fulfill(content_type='text/html',body='<link rel="stylesheet" href="/static/megaron.css"><div id="map-root" style="display:none"><canvas id="hex-canvas"></canvas></div><div id="tile-tooltip"></div><div id="march-ctx" style="display:none"></div><div id="war-body" style="width:390px;padding:12px;background:var(--bg-raised)"></div>'))
   def api(route):
    req=route.request;path=req.url.split('/api/v1/')[-1]
    if req.method=='POST':
     state['posts'].append(path)
     if path.endswith('cancel-training'):
      state['units']=[u for u in state['units'] if u['id']!='spears'];data={'pop_restored':100,'refunded':{'grain':300},'already_cancelled':False}
     else:
      state['units'].append(unit.copy());data={'pop_drawn':100,'complete_at':'2099-01-01T00:00:00Z'}
    elif path=='units':data=[{'type':'spearman','batch_men':100,'costs':{'grain':300},'pop_cost':100}]
    elif path.endswith('/units'):data={'units':state['units']}
    elif '/provinces/' in path:data={'settlement':{'buildings':[{'type':'barracks'},{'type':'shipyard'}],'can_recruit':[{'unit':'spearman','can_recruit':True}]}}
    elif path.endswith('retreat-default'):data={'retreat_pct':25}
    else:data=[]
    route.fulfill(content_type='application/json',body=json.dumps(data))
   page.route('**/api/v1/**',api)
   page.goto(base+'/fixture')
   page.evaluate('''async()=>{const {State}=await import('/static/js/megaron/state.js');State.WORLD_ID='world';State.provinceData=[{id:'province',settlement_id:'city',name:'Vasiliki',own:true,is_capital:true}];const m=await import('/static/js/megaron/ui/drawers/war.js');Object.assign(window,m);await m.loadWarDrawer();}''')
   expect(page.get_by_role('button',name='Cancel training',exact=True)).to_be_visible()
   expect(page.get_by_role('button',name='Cancel build',exact=True)).to_be_visible()
   if run==0:page.locator('#war-body').screenshot(path=str(OUT/'army-cancel.png'))
   page.get_by_role('button',name='Cancel training',exact=True).click()
   expect(page.locator('#war-unit-res')).to_contain_text('100 people returned')
   expect(page.get_by_role('button',name='Cancel training',exact=True)).to_have_count(0)
   page.get_by_role('button',name='Recruit',exact=True).click()
   page.locator('[data-recruit-type="spearman"]').click()
   expect(page.locator('#war-recruit-res')).to_contain_text('Order received: 100 Spearmen')
   if run==0:page.locator('#war-body').screenshot(path=str(OUT/'recruit-receipt.png'))
   assert state['posts']==['worlds/world/units/spears/cancel-training','worlds/world/provinces/province/recruit'],state['posts']
   assert not errors,errors
   page.close()
  browser.close()
 print('PASS: two clean real drawer/controller runs; HTTP is scripted, resource conservation separately tested against fresh PostgreSQL')
finally:server.shutdown()
