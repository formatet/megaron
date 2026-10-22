#!/usr/bin/env python3
"""Actual City drawer/controller in Firefox against isolated scripted HTTP. No live accounts."""
import functools,http.server,json,threading,copy
from pathlib import Path
from playwright.sync_api import sync_playwright,expect
ROOT=Path(__file__).resolve().parents[3]
OUT=Path(__file__).resolve().parent
class Handler(http.server.SimpleHTTPRequestHandler):
 def log_message(self,*args):pass
server=http.server.ThreadingHTTPServer(('127.0.0.1',0),functools.partial(Handler,directory=str(ROOT/'web')))
threading.Thread(target=server.serve_forever,daemon=True).start()
base=f'http://127.0.0.1:{server.server_port}'
units=[{'id':'full','type':'spearman','category':'land','status':'garrison','size':100,'crew':0,'settlement_id':'city','display_name':'1st Spearmen of Phaistos'}, {'id':'hurt','type':'spearman','category':'land','status':'garrison','size':41,'crew':0,'settlement_id':'city','display_name':'2nd Spearmen of Phaistos'}, {'id':'ship','type':'galley','category':'naval','status':'garrison','size':1,'crew':20,'settlement_id':'city','display_name':'Asterion, Galley of Phaistos'}]
proof=[]
try:
 with sync_playwright() as pw:
  browser=pw.firefox.launch()
  for width in (1000,390):
   for run in (1,2):
    page=browser.new_page(viewport={'width':width,'height':900},device_scale_factor=1)
    state={'units':copy.deepcopy(units),'posts':[],'population':1000};errors=[]
    page.on('pageerror',lambda e:errors.append(str(e)))
    page.route('**/fixture',lambda r:r.fulfill(content_type='text/html',body='''<link rel="stylesheet" href="/static/megaron.css"><div id="map-root" style="display:none"><canvas id="hex-canvas"></canvas></div><div id="tile-tooltip"></div><div id="march-ctx" style="display:none"></div><div class="drawer open" id="drawer-city"><div class="drawer-header"><span class="drawer-title" id="city-drawer-title">City</span></div><div class="drawer-body" id="city-body"></div></div>'''))
    def api(route):
     req=route.request;path=req.url.split('/api/v1/')[-1]
     if req.method=='POST':
      body=req.post_data_json;state['posts'].append(body)
      assert set(body)=={'unit_ids'} and body['unit_ids']==['hurt'],body
      selected=next(u for u in state['units'] if u['id']=='hurt');state['population']+=selected['size'];state['units']=[u for u in state['units'] if u['id']!='hurt']
      data={'pop_restored':41,'population':state['population'],'settlement_name':'Phaistos','disbanded':[{'display_name':selected['display_name'],'people_returned':41}]}
     elif path.endswith('/units'):data={'units':state['units']}
     elif path.endswith('/goods'):data=[]
     elif path.endswith('/actions'):data=[]
     elif path.endswith('/placements/options'):data={'hexes':[],'buildings':[]}
     elif path.endswith('/placements'):data={'placements':[]}
     else:data={'settlement':{'id':'city','population':state['population'],'labor_pool':state['population'],'buildings':[],'build_queue':[],'army':{},'devotion':0}}
     route.fulfill(status=200,content_type='application/json',body=json.dumps(data))
    page.route('**/api/v1/**',api)
    page.goto(base+'/fixture')
    page.evaluate('''async()=>{
     const {State}=await import('/static/js/megaron/state.js');
     const {loadCityDrawer}=await import('/static/js/megaron/ui/drawers/city.js');
     const {warDisband}=await import('/static/js/megaron/ui/drawers/war.js');window.warDisband=warDisband;
     State.WORLD_ID='fixture';State.provinceData=[{id:'province',settlement_id:'city',name:'Phaistos',own:true,is_capital:true,q:0,r:0}];State.tileData=[{q:0,r:0,terrain:'plains'}];
     await loadCityDrawer();document.querySelector('[data-tab="garnison"]').click();
    }''')
    expect(page.locator('[data-disband-unit]')).to_have_count(3)
    expect(page.locator('#city-gar-sec')).to_contain_text('Returns 41 people')
    assert page.locator('#ctab-garnison input').count()==0
    if run==2:page.screenshot(path=str(OUT/f'firefox-{width}-before.png'))
    page.locator('[data-disband-unit="hurt"]').click()
    expect(page.locator('[data-disband-unit]')).to_have_count(2)
    expect(page.locator('#war-disband-res')).to_have_text('Disbanded — returns 41 people to Phaistos.')
    expect(page.locator('#ctab-garnison')).to_be_visible()
    if run==2:page.screenshot(path=str(OUT/f'firefox-{width}-after.png'))
    assert len(state['posts'])==1 and state['population']==1041
    assert not errors,errors
    proof.append({'width':width,'run':run,'posts':state['posts'],'population':state['population'],'page_errors':errors})
    page.close()
  browser.close()
 print(json.dumps(proof,indent=2))
finally:server.shutdown();server.server_close()
