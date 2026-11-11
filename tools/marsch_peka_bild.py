#!/usr/bin/env python3
"""BILD for War -> March through pickHex: real hexpick.js/war.js/kult.js/force_card.js/megaron.css and the map.html chrome in Firefox.
The canvas is a plain stand-in grid wired to the real pickTap (map.js hands its taps to the same function); readable names,
no live game. Also records the part D rows (what covers the map at 390 px)."""
from pathlib import Path
import functools,http.server,json,re,threading
from playwright.sync_api import sync_playwright
ROOT=Path(__file__).resolve().parents[1];OUT=ROOT/'docs/reviews/marsch-peka';OUT.mkdir(parents=True,exist_ok=True)
class H(http.server.SimpleHTTPRequestHandler):
 def log_message(self,*a):pass
srv=http.server.ThreadingHTTPServer(('127.0.0.1',0),functools.partial(H,directory=str(ROOT/'web')));threading.Thread(target=srv.serve_forever,daemon=True).start()
base=f'http://127.0.0.1:{srv.server_port}'
html=re.sub(r'<script\b[^>]*>.*?</script>','',(ROOT/'web/static/map.html').read_text(),flags=re.S)
boot='''<script type="module">
import {State} from '/static/js/megaron/state.js';
import * as hp from '/static/js/megaron/ui/hexpick.js';
import * as war from '/static/js/megaron/ui/drawers/war.js';
import {renderUnitCard} from '/static/js/megaron/ui/force_card.js';
import * as mc from '/static/js/megaron/ui/marchctx.js';
import {openMobilePanel} from '/static/js/megaron/ui/mobile_panels.js';
State.WORLD_ID='world';State.TICK_SECONDS=1800;
const S=34,cv=document.getElementById('hex-canvas');
const R=[];for(let q=-6;q<=7;q++)for(let r=-6;r<=6;r++){const t=q>=3?(q>=5?'deep_sea':'coastal_sea'):(Math.abs(r)>4?'fog':(q<=-4?'hills':'plains'));R.push({q,r,terrain:t});}
State.tileData=R;State.provinceData=[{q:0,r:0,name:'Phaistos',own:true},{q:-2,r:0,name:'Knossos'}];
const U={id:'u2',type:'spearman',display_name:'2nd Spearmen of Phaistos',category:'land',status:'garrison',deployable:true,quantity:100,size:100,settlement_id:'s1',q:0,r:0};
State.unitsData=[U];window.UNIT=U;
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
Object.assign(window,{unitMarch:war.unitMarch,sendMarch:mc.sendMarch,closeMarchCtx:mc.closeMarchCtx,openMarchCtx:mc.openMarchCtx});
window.closeDrawer=(name)=>{document.getElementById('drawer-'+name).classList.remove('open');State.activeDrawer=null;};
window.openDrawerFix=(name)=>{const el=document.getElementById('drawer-'+name);el.classList.add('open');State.activeDrawer=name;openMobilePanel(el,()=>el.classList.remove('open'));};
document.getElementById('war-body').innerHTML='<div class="drawer-section">'+renderUnitCard(U)+'</div>';
window.ready=true;
</script>'''
html=html.replace('</body>',boot+'</body>')

posts=[];shots=[];CITIES=[dict(settlement_id='s1',name='Phaistos',own=True,knowledge='known',q=0,r=0,terrain='plains'),dict(settlement_id='s2',name='Knossos',owner='Minos',knowledge='known',q=-2,r=0,terrain='plains')]
def api(route):
 r=route.request;u=r.url
 if r.method=='POST':
  posts.append((u.split('/api/v1/')[-1],json.loads(r.post_data or '{}')));route.fulfill(status=200,content_type='application/json',body=json.dumps(dict(status='order_dispatched')));return
 if u.endswith('/cities'):body=CITIES
 elif u.endswith('/units'):body=dict(units=[dict(id='u2',type='spearman',display_name='2nd Spearmen of Phaistos',category='land',status='garrison',deployable=True,quantity=100,size=100,settlement_id='s1',q=0,r=0)],expedition_rules={})
 else:body={}
 route.fulfill(status=200,content_type='application/json',body=json.dumps(body))
def run(b,w,h):
 tag='desktop' if w==1280 else 'mobile'
 codes=['M1','M2','M3','M4'] if w==1280 else ['M5','M6','M7','M8']
 p=b.new_page(viewport=dict(width=w,height=h),device_scale_factor=1);errs=[]
 p.on('pageerror',lambda e:errs.append(str(e)));p.on('console',lambda m:errs.append(m.text) if m.type=='error' and 'favicon' not in m.text else None)
 p.route('https://**',lambda r:r.abort());p.route('**/fixture',lambda r:r.fulfill(content_type='text/html',body=html));p.route('**/api/v1/**',api)
 p.goto(base+'/fixture');p.wait_for_function('window.ready')
 def shot(code,name):
  p.wait_for_timeout(450);f=f'{code}-{name}-{tag}.png';p.screenshot(path=str(OUT/f));shots.append(f)
  assert p.evaluate('document.documentElement.scrollWidth<=innerWidth+1'),'sideways scroll '+f
 def visible_panels():
  return p.evaluate('''()=>[...document.querySelectorAll('.drawer,.inspect-panel,.march-ctx,.dispatch-window-overlay,.search-overlay,.codex-panel,.map-dim')].filter(e=>getComputedStyle(e).visibility!=='hidden'&&getComputedStyle(e).display!=='none'&&e.getBoundingClientRect().width>0).map(e=>e.id||e.className)''')
 def tapHex(q,r):
  x,y=p.evaluate(f'hexCenter({q},{r})');p.mouse.click(x,y)
 taps=0
 p.evaluate("openDrawerFix('war')");p.wait_for_selector('#war-body button:has-text("March")')
 p.click('#war-body button:has-text("March")');taps+=1;p.wait_for_selector('#hexpick-bar')
 assert not visible_panels(),('panel visible while picking',visible_panels())
 assert 'March 2nd Spearmen of Phaistos: tap the destination' in p.inner_text('#hexpick-bar')
 assert p.evaluate("document.getElementById('hex-canvas').inert")==False
 assert p.locator('#hexpick-bar').bounding_box()['height']<=64
 shot(codes[0],'march-pressed-bar-only')
 tapHex(2,1);taps+=1;p.wait_for_selector('[data-pick-ok]');assert 'Use this hex' in p.inner_text('[data-pick-ok]')
 shot(codes[1],'hex-selected')
 p.click('[data-pick-ok]');taps+=1;p.wait_for_selector('#march-ctx',state='visible');p.wait_for_selector('#mctx-units input[type=checkbox]')
 chk=p.evaluate("[...document.querySelectorAll('#mctx-units input[type=checkbox]')].map(c=>c.checked)");assert chk==[True],chk
 assert 'hex-picking' not in p.evaluate('document.body.className')
 if w==1280:
  bx=p.locator('#march-ctx').bounding_box();hx,hy=p.evaluate('hexCenter(2,1)')
  assert not (bx['x']<=hx<=bx['x']+bx['width'] and bx['y']<=hy<=bx['y']+bx['height']),('menu covers the chosen hex',bx,hx,hy)
 shot(codes[2],'march-menu-open-unit-checked')
 p.click('#mctx-send');taps+=1;p.wait_for_timeout(600)
 assert posts and posts[-1][0]=='worlds/world/units/u2/march' and posts[-1][1].get('target_q')==2 and posts[-1][1].get('target_r')==1,posts
 print(tag,'taps (March, hex, Use this hex, March ->):',taps,'; post',posts[-1])
 # Cities sheet
 p.evaluate("closeMarchCtx()") if False else None
 p.reload();p.wait_for_function('window.ready')
 p.evaluate("openDrawerFix('war')");p.click('#war-body button:has-text("March")');p.wait_for_selector('[data-pick-cities]')
 p.click('[data-pick-cities]');p.wait_for_selector('#hexpick-sheet:not([hidden])')
 sh=p.locator('#hexpick-sheet').bounding_box();assert sh['height']<=0.4*h+1,sh
 assert 'Phaistos — your city' in p.inner_text('#hexpick-sheet') and 'Knossos — Minos' in p.inner_text('#hexpick-sheet')
 assert not visible_panels()
 shot(codes[3],'cities-sheet')
 p.click('#hexpick-sheet button:has-text("Knossos")');p.wait_for_selector('#march-ctx',state='visible')
 assert 'Knossos' in p.inner_text('#mctx-name')
 assert not errs,errs
 p.close()
with sync_playwright() as pw:
 b=pw.firefox.launch()
 for w,h in [(1280,800),(390,844)]:run(b,w,h)
 b.close()
print('\n'.join(shots))
