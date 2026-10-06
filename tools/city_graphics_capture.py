from pathlib import Path
import json, sys
from playwright.sync_api import sync_playwright
ROOT=Path(__file__).resolve().parents[1]
# Existing exported geography with explicit synthetic objects; no game server.
OUT=Path(sys.argv[1])
OUT.mkdir(parents=True,exist_ok=True)
HOST=sys.argv[2] if len(sys.argv)>2 else 'http://127.0.0.1:18199'
fx=json.loads((ROOT/'web/static/fixtures/world-full.json').read_text())
ts={(t['q'],t['r']):t for t in fx['tiles']}
city=next(p for p in fx['provinces'] if p['name']=='Amyklai')
city.update(own=True,is_capital=True,walls=1,size_tier=1,state="active")
cq,cr=city['q'],city['r']
offs=sorted([(q,r) for q in range(-2,3) for r in range(-2,3) if 0<max(abs(q),abs(r),abs(q+r))<=2],key=lambda v:(max(abs(v[0]),abs(v[1]),abs(sum(v))),v))
hexes=[]
for i,(dq,dr) in enumerate(offs,1):
 t=ts[cq+dq,cr+dr];good={'plains':'grain','hills':'livestock','forest_olive_grove':'olives','mountain_limestone':'stone','coastal_sea':'fish','deep_sea':'fish'}.get(t['terrain'],'grain')
 placed=3 if (dq,dr)==(1,0) else 0
 hexes.append(dict(hex_ordinal=i,hex_q=cq+dq,hex_r=cr+dr,terrain=t['terrain'],goods=[dict(good_key=good,cap=6,placed=placed,placed_ordinals=list(range(placed)),rate_per_tick=placed*2.,marginal_yield=2.)]))
blds=[dict(type='farm',level=1,hex_q=10,hex_r=32),dict(type='farm',level=2,hex_q=10,hex_r=31),dict(type='market',level=1),dict(type='barracks',level=1),dict(type='harbour',level=1)]
sett=dict(id=city['settlement_id'],population=1000,labor_pool=10,walls=1,buildings=blds,build_queue=[],training_units=[],army={},loyalty=4)
opts=dict(hexes=hexes,buildings=[],total_gubbar=10,pool_size=7)
units=[]
# Real geography, synthesized positioned actors: separated within the same land region.
for kind,q,r in [('nomadic_host',12,31),('spearman',11,32),('elite_infantry',12,30),('war_chariot',13,30),('runner',13,32),('galley',10,33),('war_galley',11,33),('merchantman',12,33)]:
 # Only choose sea hexes for naval; nearest sea if nominal location is land.
 naval=kind in ['galley','war_galley','merchantman']
 if naval and ts[q,r]['terrain'] not in ['coastal_sea','deep_sea']:
  cand=[t for t in fx['tiles'] if t['terrain'] in ['coastal_sea','deep_sea'] and (t['q'],t['r']) not in [(u['q'],u['r']) for u in units]]
  t=min(cand,key=lambda t:abs(t['q']-q)+abs(t['r']-r));q,r=t['q'],t['r']
 assert (ts[q,r]['terrain'] in ['coastal_sea','deep_sea'])==naval
 units.append(dict(id='fixture-'+kind,type=kind,category='naval' if naval else 'land',q=q,r=r,status='positioned',size=100))
rural=[dict(building_type='farm',q=b['hex_q'],r=b['hex_r'],level=b['level']) for b in blds if b['type']=='farm']
for rp in rural:
 assert ts[rp['q'],rp['r']]['terrain']=='plains'
 assert max(abs(rp['q']-cq),abs(rp['r']-cr),abs(rp['q']+rp['r']-cq-cr))<=2
assert any(ts.get((cq+dq,cr+dr),{}).get('terrain')=='coastal_sea' for dq,dr in [(1,0),(0,1),(-1,1),(-1,0),(0,-1),(1,-1)])
prov=[dict(p,own=p['id']==city['id']) for p in fx['provinces']]
prov=[city if p['id']==city['id'] else p for p in prov]
payload=dict(city=city,settlement=sett,placement_options=opts,units=units,rural=rural)
(OUT/'fixture-additions.json').write_text(json.dumps(payload,indent=2))
errors=[];metrics={}
with sync_playwright() as p:
 b=p.chromium.launch()
 page=b.new_page(viewport={'width':1200,'height':900},device_scale_factor=1)
 page.on('pageerror',lambda e:errors.append(str(e)))
 page.goto(HOST+'/static/showcase-world.html?zoom=1',wait_until='networkidle')
 page.wait_for_function('window.SHOWCASE?.ready')
 page.evaluate('''async data=>{window.S= (await import('/static/js/megaron/state.js')).State; window.M=await import('/static/js/megaron/render/map.js'); S.provinceData=data.prov; S.unitsData=data.units;S.ruralData=data.rural;S.workedHexes=[{q:10,r:32}]; const root=document.querySelector('#map-root');root.style.width='1200px';root.style.height='900px'; M.canvas.width=2400;M.canvas.height=1800;window.focus=(q,r)=>{const p=M.hexPx(q,r);S.camera.zoom=1;S.camera.x=M.canvas.width/2-p.x*M.SCALE;S.camera.y=M.canvas.height/2-p.y*M.SCALE;SHOWCASE.draw(100);};}''',dict(prov=prov,units=units,rural=rural))
 def shot(name,q,r,w=800,h=550):
  page.evaluate('([q,r])=>focus(q,r)',[q,r]);page.screenshot(path=str(OUT/(name+'.png')),clip={'x':600-w/2,'y':450-h/2,'width':w,'height':h})
  bounds=page.evaluate("""async ([w,h])=>{
   const C=await import('/static/js/megaron/render/citysprites.js');
   const k=S.camera.zoom*M.SCALE, css=M.canvas.getBoundingClientRect();
   const sx=css.width/M.canvas.width, sy=css.height/M.canvas.height;
   return S.provinceData.filter(p=>p.size_tier===1 && p.state==='active').map(p=>{
     const a=M.hexPx(p.q,p.r), sprite=C.citySprite(p.size_tier,p.walls||0);
     const x=(S.camera.x+a.x*k)*sx-(600-w/2);
     const y=(S.camera.y+a.y*k)*sy-(450-h/2);
     return [Math.floor(x-(sprite.w/2+3)*k*sx),
       Math.floor(y+(C.cityTop(sprite)-4)*k*sy),
       Math.ceil(x+(sprite.w/2+3)*k*sx),
       Math.ceil(y+(C.cityTop(sprite)+sprite.h+4)*k*sy)];
   });
  }""",[w,h])
  metrics[name]=dict(zoom=1,dpr=1,center=[q,r],width=w,height=h,city_bounds=bounds)
 shot('02-farmer-kuststad',10,32,700,450)
 shot('03-enheter-host',12,31,800,500)
 shot('04a-terrang-vast',8,28,900,650)
 shot('04b-terrang-ost',43,38,900,650)
 shot('05a-kartstader-kust',42,35,800,550)
 shot('05b-kartstader-inland',46,38,700,450)
 shot('02-farmer-kuststad-repeat',10,32,700,450)
 # Native production City drawer; API data are explicit local fixtures, no server.
 def route(route):
  url=route.request.url
  if url.endswith('/placement-options'):data=opts
  elif url.endswith('/goods'):data=[dict(key='grain',amount=200,rate_per_tick=6,idle_citizens=7)]
  elif url.endswith('/loyalty-log'):data=[]
  elif '/provinces/' in url:data={'settlement':sett}
  else:data=[]
  route.fulfill(status=200,content_type='application/json',body=json.dumps(data))
 page.route('**/api/v1/**',route)
 page.evaluate('''()=>{document.body.insertAdjacentHTML('beforeend','<div id="drawer-city" class="drawer open"><div class="drawer-header"><span id="city-drawer-title" class="drawer-title">City</span></div><div id="city-body" class="drawer-body"></div></div>');window.requestAnimationFrame=fn=>{window.cityFrame=fn;return 0;};window.cancelAnimationFrame=()=>{};}''')
 page.evaluate('''async()=>{const C=await import('/static/js/megaron/ui/drawers/city.js');await C.loadCityDrawer();if(window.cityFrame)window.cityFrame(0);(await import('/static/js/megaron/render/city.js')).stopCityAnim();}''')
 page.wait_for_selector('#city-gubbe-grid .gubbe-hex-g')
 page.locator('#drawer-city').screenshot(path=str(OUT/'01a-city-drawer.png'))
 page.locator('#city-scene').screenshot(path=str(OUT/'01b-city-scene.png'))
 page.locator('#city-gubbe-grid').screenshot(path=str(OUT/'01c-citygrid.png'))
 ordinal=next(h['hex_ordinal'] for h in hexes if (h['hex_q'],h['hex_r'])==(10,32))
 page.locator(f'.gubbe-hex-g[data-ordinal="{ordinal}"]').click()
 page.locator('#city-gubbe-grid').screenshot(path=str(OUT/'01d-citygrid-farm-selected.png'))
 metrics['city_scene']=page.locator('#city-scene').evaluate('(c)=>({cssWidth:c.getBoundingClientRect().width,cssHeight:c.getBoundingClientRect().height,canvasWidth:c.width,canvasHeight:c.height,dpr:devicePixelRatio})')
 b.close()
(OUT/'capture-metadata.json').write_text(json.dumps(dict(metrics=metrics,errors=errors,source='world-full historical exported geography; current assets; synthesized visual scenarios',geography_checks='PASS farms on plains and within radius 2, coastal city adjacent to coastal sea, land/naval units on appropriate terrain'),indent=2))
print(json.dumps({'errors':errors,'output':str(OUT)},indent=2))
if errors: sys.exit(1)
