#!/usr/bin/env python3
"""BILD for pointing at the map: real hexpick.js/war.js/kult.js/force_card.js/megaron.css and the map.html chrome in Firefox.
The canvas is a plain stand-in grid wired to the real pickTap (map.js hands its taps to the same function); readable names,
no live game. Also records the part D rows (what covers the map at 390 px)."""
from pathlib import Path
import functools,http.server,json,re,threading
from playwright.sync_api import sync_playwright
ROOT=Path(__file__).resolve().parents[1];OUT=ROOT/'docs/reviews/peka-pa-kartan';OUT.mkdir(parents=True,exist_ok=True)
class H(http.server.SimpleHTTPRequestHandler):
 def log_message(self,*a):pass
srv=http.server.ThreadingHTTPServer(('127.0.0.1',0),functools.partial(H,directory=str(ROOT/'web')));threading.Thread(target=srv.serve_forever,daemon=True).start()
base=f'http://127.0.0.1:{srv.server_port}'
html=re.sub(r'<script\b[^>]*>.*?</script>','',(ROOT/'web/static/map.html').read_text(),flags=re.S)
boot='''<script type="module">
import {State} from '/static/js/megaron/state.js';
import * as hp from '/static/js/megaron/ui/hexpick.js';
import * as war from '/static/js/megaron/ui/drawers/war.js';
import * as kult from '/static/js/megaron/ui/drawers/kult.js';
import {offerRowHTML} from '/static/js/megaron/ui/drawers/kult_offer.js';
import {renderUnitCard} from '/static/js/megaron/ui/force_card.js';
import {openMobilePanel} from '/static/js/megaron/ui/mobile_panels.js';
State.WORLD_ID='world';State.TICK_SECONDS=1800;
const S=34,cv=document.getElementById('hex-canvas'),root=document.getElementById('map-root');
const R=[];for(let q=-6;q<=7;q++)for(let r=-6;r<=6;r++){const t=q>=3?(q>=5?'deep_sea':'coastal_sea'):(Math.abs(r)>4?'fog':(q<=-4?'hills':'plains'));R.push({q,r,terrain:t});}
State.tileData=R;State.provinceData=[{q:0,r:0,name:'Phaistos',own:true},{q:-2,r:0,name:'Knossos'}];
State.unitsData=[{id:'u1',type:'spearman',display_name:'2nd Spearmen of Phaistos',category:'land',status:'marching',target_q:4,target_r:2,size:100}];
const px=(q,r,w,h)=>[w/2+S*1.5*q,h/2+S*(Math.sqrt(3)/2*q+Math.sqrt(3)*r)];
const at=(x,y,w,h)=>{const wx=x-w/2,wy=y-h/2,q=(2/3*wx)/S,r=(-wx/3+Math.sqrt(3)/3*wy)/S,s=-q-r;let a=Math.round(q),b=Math.round(r),c=Math.round(s);const dq=Math.abs(a-q),dr=Math.abs(b-r),ds=Math.abs(c-s);if(dq>dr&&dq>ds)a=-b-c;else if(dr>ds)b=-a-c;return {q:a,r:b};};
const col={plains:'#b9c27a',hills:'#a08a5a',coastal_sea:'#6aa5c8',deep_sea:'#3f7fae',fog:'#2b2b2b'};
function draw(){const w=cv.clientWidth,h=cv.clientHeight;if(cv.width!==w||cv.height!==h){cv.width=w;cv.height=h;}const c=cv.getContext('2d');c.clearRect(0,0,w,h);
 for(const t of R){const [x,y]=px(t.q,t.r,w,h);c.beginPath();for(let i=0;i<6;i++){const a=Math.PI/3*i;c.lineTo(x+S*Math.cos(a),y+S*Math.sin(a));}c.closePath();c.fillStyle=col[t.terrain];c.fill();c.strokeStyle='#0003';c.stroke();
  const s=State.pickHexSel;if(s&&s.q===t.q&&s.r===t.r){c.strokeStyle='#F9E79F';c.lineWidth=3;c.stroke();c.lineWidth=1;}}
 for(const p of State.provinceData){const [x,y]=px(p.q,p.r,w,h);c.fillStyle='#4a2c1a';c.fillRect(x-7,y-7,14,14);}}
setInterval(draw,50);
cv.addEventListener('pointerup',e=>{const r=cv.getBoundingClientRect();const h=at(e.clientX-r.left,e.clientY-r.top,r.width,r.height);if(hp.pickActive())hp.pickTap(h.q,h.r);});
window.hexCenter=(q,r)=>{const b=cv.getBoundingClientRect();const [x,y]=px(q,r,b.width,b.height);return [b.left+x,b.top+y];};
Object.assign(window,{unitRedirectToggle:war.unitRedirectToggle,unitRedirect:war.unitRedirect,pickStormHex:kult.pickStormHex});
window.openDrawerFix=(name)=>{const el=document.getElementById('drawer-'+name);el.classList.add('open');openMobilePanel(el,()=>el.classList.remove('open'));};
window.closeDrawerFix=(name)=>{document.getElementById('drawer-'+name).classList.remove('open');};
const wake={id:'minoan_stillness_wake',god:'The Sea Daimons',god_id:'sea_daimons_stillness',offering:{fish:10,wine:6},targets:{unit:[],city:{count:0},realm:{count:0},kind:'hex'}};
document.getElementById('war-body').innerHTML='<div class="drawer-section">'+renderUnitCard(State.unitsData[0])+'<div id="uorder-u1"></div></div>';
document.getElementById('kult-body').innerHTML='<div class="drawer-section"><h3>Prayers</h3><div class="offer-row" id="offer-row-w">'+offerRowHTML(wake,{fish:20,wine:20})+'</div></div>';
window.showPanel=(id,mode)=>{const el=document.getElementById(id);if(mode==='display')el.style.display='block';else el.classList.add('open');openMobilePanel(el,()=>{});};
window.ready=true;
</script>'''
html=html.replace('</body>',boot+'</body>')
posts=[]
def api(route):
 r=route.request
 if r.method=='POST':posts.append((r.url.split('/api/v1/')[-1],json.loads(r.post_data)))
 route.fulfill(status=200,content_type='application/json',body=json.dumps(dict(status='order_dispatched')))
shots=[]
def run(b,w,h):
 tag='desktop' if w==1280 else 'mobile'
 codes=['K1','K2','K3','K4','K5'] if w==1280 else ['K6','K7','K8','K9','K10']
 p=b.new_page(viewport=dict(width=w,height=h),device_scale_factor=1);errs=[]
 p.on('pageerror',lambda e:errs.append(str(e)));p.on('console',lambda m:errs.append(m.text) if m.type=='error' and 'favicon' not in m.text else None)
 p.route('https://**',lambda r:r.abort());p.route('**/fixture',lambda r:r.fulfill(content_type='text/html',body=html));p.route('**/api/v1/**',api)
 p.goto(base+'/fixture');p.wait_for_function('window.ready')
 def shot(code,name):
  p.wait_for_timeout(350);f=f'{code}-{name}-{tag}.png';p.screenshot(path=str(OUT/f));shots.append(f)
  assert p.evaluate('document.documentElement.scrollWidth<=innerWidth+1'),'sideways scroll '+f
 def check_picking():
  # nothing that could cover the map is visible, the bar is short, and the canvas takes taps
  hid=p.evaluate('''()=>[...document.querySelectorAll('.drawer,.inspect-panel,.march-ctx,.dispatch-window-overlay,.search-overlay,.codex-panel,.map-dim')].filter(e=>getComputedStyle(e).visibility!=='hidden').map(e=>e.id||e.className)''')
  assert not hid,('still visible while picking',hid)
  bar=p.locator('#hexpick-bar').bounding_box();assert bar['height']<=64,('bar too tall',bar)
  assert p.evaluate("document.getElementById('hex-canvas').inert")==False
  return bar['height']
 def tapHex(q,r):
  x,y=p.evaluate(f'hexCenter({q},{r})');p.mouse.click(x,y)
 # ── Redirect ──
 p.evaluate("openDrawerFix('war')");p.wait_for_selector('#war-body button:has-text("Redirect")')
 assert p.evaluate("document.getElementById('uredir-q-u1').value")==''
 p.click('#war-body button:has-text("Redirect")');p.wait_for_selector('#hexpick-bar')
 bh=check_picking();shot(codes[0],'redirect-pressed-bar-no-drawer')
 tapHex(-2,0);p.wait_for_selector('[data-pick-ok]');assert 'Knossos' in p.inner_text('[data-pick-ok]') and 'Use this hex' in p.inner_text('[data-pick-ok]')
 assert p.locator('#hexpick-bar').bounding_box()['height']<=64
 shot(codes[1],'hex-selected-use-this-hex')
 p.click('[data-pick-ok]');p.wait_for_selector('#hexpick-bar',state='detached')
 assert p.evaluate("getComputedStyle(document.getElementById('drawer-war')).visibility")=='visible'
 txt=p.inner_text('#uredir-picked-u1');assert 'Redirect to (-2,0)' in txt and 'Send order' in txt,txt
 shot(codes[2],'back-on-card-send-order')
 p.click('#uredir-picked-u1 button');p.wait_for_function('document.getElementById("uorder-u1").textContent!==""')
 assert posts[-1]==('worlds/world/units/u1/recall',dict(target_q=-2,target_r=0)),posts[-1]
 # empty Q/R is refused client-side
 p.evaluate("document.getElementById('uredir-q-u1').value='';document.getElementById('uredir-r-u1').value=''");n=len(posts)
 p.evaluate("unitRedirect('u1')");p.wait_for_timeout(150);assert len(posts)==n
 assert p.inner_text('#uorder-u1')=='Type both Q and R.'
 p.evaluate("closeDrawerFix('war')")
 # ── Wake the Storm ──
 p.evaluate("openDrawerFix('kult')");p.wait_for_selector('[data-pick-storm-hex]')
 p.click('[data-pick-storm-hex]');p.wait_for_selector('#hexpick-bar');check_picking()
 tapHex(0,1);p.wait_for_selector('.hexpick-reason');assert 'Not a sea hex' in p.inner_text('.hexpick-reason');assert p.locator('[data-pick-ok]').count()==0
 shot(codes[3],'refused-hex-not-sea')
 tapHex(3,-1);p.wait_for_selector('[data-pick-ok]');p.click('[data-pick-ok]');p.wait_for_selector('#hexpick-bar',state='detached')
 assert p.input_value('.offer-target-hex')=='3,-1'
 assert p.evaluate("getComputedStyle(document.getElementById('drawer-kult')).visibility")=='visible'
 shot(codes[4],'wake-the-storm-field-filled')
 # Escape cancels and restores
 p.click('[data-pick-storm-hex]');p.wait_for_selector('#hexpick-bar');p.keyboard.press('Escape') if False else p.click('[data-pick-cancel]')
 p.wait_for_selector('#hexpick-bar',state='detached');assert p.input_value('.offer-target-hex')=='3,-1'
 assert not errs,errs
 print(tag,'bar height',bh)
 p.close()
# ── Part D: what covers the map at 390 x 844 (report only) ────────────────────────────────────────────────────────────
ROWS=[('P1','March here / the march menu (map tap or right-click)','march-ctx','display','#mctx-name'),
 ('P2','Colonize (a checkbox inside the same march menu)','march-ctx','display','#mctx-colonize-chk'),
 ('P3','"Show ..." from a dispatch (opens the hex panel)','inspect-panel','display','#ip-name'),
 ('P4','Clicking a chip (dispatch window)','dispatch-window-overlay','open','.dw-body'),
 ('P5','Send goods -> (Economy drawer, send tab)','drawer-economy','open','#economy-body'),
 ('P6','Build pick mode (City drawer grid)','drawer-city','open','#city-body'),
 ('P7','War -> March (March menu says "Choose a destination on the map")','march-ctx','display','#mctx-hint')]
def part_d(b):
 out=[]
 for code,flow,eid,mode,_ in ROWS:
  p=b.new_page(viewport=dict(width=390,height=844),device_scale_factor=1)
  p.route('https://**',lambda r:r.abort());p.route('**/fixture',lambda r:r.fulfill(content_type='text/html',body=html));p.route('**/api/v1/**',api)
  p.goto(base+'/fixture');p.wait_for_function('window.ready')
  if code=='P7':p.evaluate("document.getElementById('mctx-name').textContent='March';document.getElementById('mctx-hint').textContent='Choose a destination on the map.'")
  p.evaluate(f"showPanel('{eid}','{mode}')");p.wait_for_timeout(300)
  m=p.evaluate(f'''()=>{{const e=document.getElementById('{eid}'),r=e.getBoundingClientRect(),c=document.getElementById('hex-canvas').getBoundingClientRect();
   const ix=Math.max(0,Math.min(r.right,c.right)-Math.max(r.left,c.left)),iy=Math.max(0,Math.min(r.bottom,c.bottom)-Math.max(r.top,c.top));
   return {{cover:Math.round(100*ix*iy/(c.width*c.height)),inert:document.getElementById('hex-canvas').inert,blocked:document.getElementById('map-root').classList.contains('mobile-panel-blocked')}}}}''')
  f=f'{code}-covers-map-mobile.png';p.screenshot(path=str(OUT/f));shots.append(f);out.append((code,flow,m,f));p.close()
 (OUT/'part_d.json').write_text(json.dumps(out,indent=1))
 for o in out:print(o)
with sync_playwright() as pw:
 b=pw.firefox.launch()
 for w,h in [(1280,800),(390,844)]:run(b,w,h)
 part_d(b)
 b.close()
print('\n'.join(shots))
