#!/usr/bin/env python3
"""Actual Firefox place naming/label BILD fixture (explicit SQL rights + FOW).
Layout data is not evidence of natural first reach; movement proof is separate.
"""
import json,pathlib,subprocess,urllib.request
from playwright.sync_api import sync_playwright
ROOT=pathlib.Path(__file__).resolve().parents[1];OUT=ROOT/'docs/reviews/flod-doparratt/pictures';OUT.mkdir(parents=True,exist_ok=True);BASE='http://localhost:8097';PW='acceptance-pw-123'
def api(path,token=None,data=None):
 req=urllib.request.Request(BASE+'/api/v1/'+path,headers={'Content-Type':'application/json',**({'Authorization':'Bearer '+token} if token else {})},data=None if data is None else json.dumps(data).encode());return json.load(urllib.request.urlopen(req,timeout=30))
health=json.load(urllib.request.urlopen(BASE+'/healthz'));expected=subprocess.check_output(['git','-C',str(ROOT),'rev-parse','--short=7','HEAD'],text=True).strip();assert health['commit']==expected and health['migration']==172,health
world=api('worlds')[0]['id'];token=api('auth/login',data={'username_or_email':'Eirene','password':PW})['access_token'];player=api('auth/me',token)['id']
# Explicit BILD remembered terrain on Eirene permits showing all
# four natural component types without claiming this was a discovery playtest.
subprocess.run(['tools/acceptance.sh','psql',f"INSERT INTO player_scouted_tiles(world_id,player_id,q,r) SELECT world_id,'{player}',q,r FROM map_tiles WHERE world_id='{world}' ON CONFLICT DO NOTHING"],cwd=ROOT,check=True,capture_output=True)
places=api(f'worlds/{world}/places',token)['places'];names={'river':'Thaleia','massif':'Mount Ida','desert':'Damon Sands','forest':'Ariadne Cedars'};selected=[]
for kind,name in names.items():
 p=next(p for p in places if p['kind']==kind);q,r=map(int,p['id'].split(','))
 subprocess.run(['tools/acceptance.sh','psql',f"INSERT INTO place_names(world_id,kind,anchor_q,anchor_r,claimed_by,claimed_tick) VALUES('{world}','{kind}',{q},{r},'{player}',(SELECT current_tick FROM worlds WHERE id='{world}')) ON CONFLICT(world_id,kind,anchor_q,anchor_r) DO UPDATE SET claimed_by=EXCLUDED.claimed_by,name=NULL,named_tick=NULL"],cwd=ROOT,check=True,capture_output=True)
 p['human_name']=name;selected.append(p)
# Pick only known actual component hexes nearest their centre.
for p in selected:
 cx=sum(h['q'] for h in p['hexes'])/len(p['hexes']);cy=sum(h['r'] for h in p['hexes'])/len(p['hexes']);p['focus']=min(p['hexes'],key=lambda h:(h['q']-cx)**2+(h['r']-cy)**2)
facts=[]
with sync_playwright() as pw:
 browser=pw.firefox.launch()
 for mode,(w,h) in {'desktop':(1280,900),'mobile':(390,844)}.items():
  for variant in ['a','b']:
   if variant=='a':
    for p in selected:
     q,r=map(int,p['id'].split(','));subprocess.run(['tools/acceptance.sh','psql',f"UPDATE place_names SET name=NULL,named_tick=NULL WHERE world_id='{world}' AND kind='{p['kind']}' AND anchor_q={q} AND anchor_r={r}"],cwd=ROOT,check=True,capture_output=True)
   ctx=browser.new_context(viewport={'width':w,'height':h},has_touch=mode=='mobile');ctx.add_cookies([{'name':'poleia_token','value':token,'url':BASE}]);ctx.add_init_script(f"localStorage.setItem('poleia_token',{json.dumps(token)})")
   page=ctx.new_page();errors=[];page.on('pageerror',lambda e:errors.append(str(e)));page.goto(BASE+'/world/'+world+'/map?ortnamn='+variant,wait_until='networkidle')
   page.evaluate("""async()=>{window.__ps=(await import('/static/js/megaron/state.js')).State;window.__pm=await import('/static/js/megaron/render/map.js');window.__pl=await import('/static/js/megaron/render/place_names.js');}""")
   page.wait_for_function('__ps.tileData.length>0 && __pm.markrasterStats.pending===0');assert page.evaluate('__pl.placeLabelVariant(location.search)')==variant,page.url
   for p in selected:
    page.evaluate('h=>centreOn(h.q,h.r)',p['focus']);page.wait_for_function('__pm.markrasterStats.pending===0');page.wait_for_timeout(400)
    if variant=='a':
     point=page.evaluate('h=>{const p=__pm.hexPx(h.q,h.r);return {x:__ps.camera.x+p.x*__pm.SCALE*__ps.camera.zoom,y:__ps.camera.y+p.y*__pm.SCALE*__ps.camera.zoom}}',p['focus'])
     canvas=page.locator('#hex-canvas');canvas.tap(position=point) if mode=='mobile' else canvas.click(position=point);page.locator('[aria-label="Place name"]').wait_for();page.screenshot(path=str(OUT/f'{mode}-{p["kind"]}-form.png'));page.locator('[aria-label="Place name"]').fill(p['human_name']);page.locator('#ip-foot button').filter(has_text='Name it').click();page.locator('[aria-label="Place name"]').wait_for(state='hidden');page.keyboard.press('Escape');page.evaluate('__pm.closeInspect()')
    else:
     # Already named by the first desktop web sequence; reread real map.
     page.evaluate('async()=>{await window.refreshTiles();}');page.wait_for_timeout(500)
    page.evaluate('h=>centreOn(h.q,h.r)',p['focus']);page.wait_for_function('__pm.markrasterStats.pending===0');page.wait_for_timeout(500)
    page.screenshot(path=str(OUT/f'{mode}-{variant}-{p["kind"]}-label.png'))
    measured=page.evaluate("""variant=>{const c=document.createElement('canvas').getContext('2d'),style=__pl.PLACE_LABEL_VARIANTS[variant],z=__ps.camera.zoom*__pm.SCALE;return __pl.placeLabelLayout(__ps.tileData,__ps.provinceData,{hexPx:__pm.hexPx,measure:t=>{c.font=style.font;return style.spacing?[...t].reduce((n,ch)=>n+c.measureText(ch).width,0):c.measureText(t).width},measureCity:(t,own)=>{c.font=own?'bold 7px monospace':'6px monospace';return c.measureText(t).width},variant,zoom:__ps.camera.zoom,viewport:{left:-__ps.camera.x/z,top:-__ps.camera.y/z,right:(innerWidth-__ps.camera.x)/z,bottom:(document.getElementById('hex-canvas').height-__ps.camera.y)/z}})}""",variant)
    assert any(l['name']==p['human_name'] for l in measured),(mode,variant,p,measured)
    facts.append({'mode':mode,'variant':variant,'url':page.url,'kind':p['kind'],'labels':measured,'errors':list(errors)})
   ctx.close()
 browser.close()
(OUT/'facts.json').write_text(json.dumps({'health':health,'world':world,'fixture':'explicit SQL rights and remembered geography; actual web name POST','places':selected,'measurements':facts},indent=2)+'\n')
assert all(not f['errors'] for f in facts),facts
print('Firefox two variants, four kinds, desktop+mobile: labels measured and web naming proven')
