#!/usr/bin/env python3
"""Chosen italic label BILD with real server LIVE sight from explicit SQL units.
Fixture units/previously named objects prove layout, not natural discovery.
"""
import json
import pathlib
import subprocess
import urllib.request
from playwright.sync_api import sync_playwright

ROOT = pathlib.Path(__file__).resolve().parents[1]
OUT = ROOT / 'docs/reviews/flod-doparratt/live-labels'
OUT.mkdir(parents=True, exist_ok=True)
BASE = 'http://localhost:8097'

def api(path, token=None, data=None):
    req = urllib.request.Request(BASE+'/api/v1/'+path, headers={
        'Content-Type':'application/json', **({'Authorization':'Bearer '+token} if token else {})},
        data=None if data is None else json.dumps(data).encode())
    return json.load(urllib.request.urlopen(req, timeout=30))

def sql(query):
    return subprocess.check_output(['tools/acceptance.sh', 'psql', query], cwd=ROOT, text=True)

def distance(a,b):
    dq=a['q']-b['q'];dr=a['r']-b['r']
    return max(abs(dq),abs(dr),abs(dq+dr))

health=json.load(urllib.request.urlopen(BASE+'/healthz'))
expected=subprocess.check_output(['git','-C',str(ROOT),'rev-parse','--short=7','HEAD'],text=True).strip()
assert health['commit']==expected and health['migration']==172, health
world=api('worlds')[0]['id']
token=api('auth/login',data={'username_or_email':'Eirene','password':'acceptance-pw-123'})['access_token']
player=api('auth/me',token)['id']
old=json.loads((ROOT/'docs/reviews/flod-doparratt/pictures/facts.json').read_text())
selected=old['places']
tiles=json.loads(sql(f"SELECT json_agg(t) FROM (SELECT q,r,terrain FROM map_tiles WHERE world_id='{world}') t"))
blocked={'coastal_sea','deep_sea','river','mountain_limestone','mountain_red'}
mountains={'mountain_limestone','mountain_red'}
# Greedy minimum coverage by genuine positioned infantry on passable land.
# No client tier overrides: normal API FOW evaluates these real live eyes.
eyes={}
for p in selected:
    targets=[t for t in tiles if distance(t,p['focus'])<=8 and t['terrain'] not in {'coastal_sea','deep_sea'}]
    candidates=[t for t in tiles if distance(t,p['focus'])<=10 and t['terrain'] not in blocked and distance(t,p['focus'])>=2]
    unseen=list(targets)
    while unseen:
        def sees(a,b):return distance(a,b)<=(4 if b['terrain'] in mountains else 2)
        c=max(candidates,key=lambda c:sum(sees(c,t) for t in unseen))
        seen=[t for t in unseen if sees(c,t)]
        if not seen:break
        eyes[(c['q'],c['r'])]=c
        unseen=[t for t in unseen if not sees(c,t)]

ids=[];facts=[]
try:
    for i,t in enumerate(eyes.values()):
        raw=sql(f"INSERT INTO units(world_id,owner_id,type,category,size,status,q,r,name) VALUES('{world}','{player}','infantry','land',100,'positioned',{t['q']},{t['r']},'BILD live scout {i}') RETURNING id").strip()
        ids.append(raw.splitlines()[0])
    with sync_playwright() as pw:
        browser=pw.firefox.launch()
        for mode,(width,height) in {'desktop':(1280,900),'mobile':(390,844)}.items():
            ctx=browser.new_context(viewport={'width':width,'height':height},has_touch=mode=='mobile')
            ctx.add_cookies([{'name':'poleia_token','value':token,'url':BASE}])
            ctx.add_init_script(f"localStorage.setItem('poleia_token',{json.dumps(token)})")
            page=ctx.new_page();errors=[]
            page.on('pageerror',lambda e:errors.append(str(e)))
            page.goto(BASE+'/world/'+world+'/map',wait_until='networkidle')
            page.evaluate("""async()=>{window.__ps=(await import('/static/js/megaron/state.js')).State;window.__pm=await import('/static/js/megaron/render/map.js');window.__pl=await import('/static/js/megaron/render/place_names.js');}""")
            page.wait_for_function('__ps.tileData.length>0 && __pm.markrasterStats.pending===0')
            for p in selected:
                page.evaluate('h=>centreOn(h.q,h.r)',p['focus'])
                page.wait_for_function('__pm.markrasterStats.pending===0');page.wait_for_timeout(500)
                measured=page.evaluate("""()=>{const c=document.createElement('canvas').getContext('2d'),style=__pl.PLACE_LABEL_STYLE,z=__ps.camera.zoom*__pm.SCALE;const labels=__pl.placeLabelLayout(__ps.tileData,__ps.provinceData,{hexPx:__pm.hexPx,measure:t=>{c.font=style.font;return c.measureText(t).width},measureCity:(t,own)=>{c.font=own?'bold 7px monospace':'6px monospace';return c.measureText(t).width},zoom:__ps.camera.zoom,viewport:{left:-__ps.camera.x/z,top:-__ps.camera.y/z,right:(innerWidth-__ps.camera.x)/z,bottom:(document.getElementById('hex-canvas').height-__ps.camera.y)/z}});const visible=__ps.tileData.filter(t=>{const p=__pm.hexPx(t.q,t.r),x=__ps.camera.x+p.x*z,y=__ps.camera.y+p.y*z;return x>0&&x<innerWidth&&y>44&&y<innerHeight-36;});return {labels,visible:visible.map(t=>({q:t.q,r:t.r,terrain:t.terrain,tier:t.tier})),zoom:__ps.camera.zoom};}""")
                label=next(l for l in measured['labels'] if l['name']==p['human_name'])
                group=page.evaluate("""p=>__ps.tileData.filter(t=>t.place_kind===p.kind&&t.place_id===p.id).map(t=>({q:t.q,r:t.r,tier:t.tier,terrain:t.terrain}))""",p)
                live=[t for t in group if t['tier']=='live']
                assert live and any(t['q']==p['focus']['q'] and t['r']==p['focus']['r'] for t in live),(p,group)
                visible_land=[t for t in measured['visible'] if t['terrain'] not in {'coastal_sea','deep_sea','fog'}]
                live_land=sum(t['tier']=='live' for t in visible_land)
                assert live_land/len(visible_land)>.7,(mode,p['kind'],live_land,len(visible_land))
                page.screenshot(path=str(OUT/f'{mode}-{p["kind"]}.png'))
                facts.append({'mode':mode,'kind':p['kind'],'url':page.url,'label':label,'group':group,'live_land':live_land,'visible_land':len(visible_land),'measurements':measured,'errors':list(errors)})
            ctx.close()
        browser.close()
    assert all(not f['errors'] for f in facts),facts
    (OUT/'facts.json').write_text(json.dumps({'health':health,'world':world,'fixture':'explicit SQL positioned land scouts on passable terrain; existing SQL naming fixture; normal server LIVE FOW, no client overrides','units':list(eyes.values()),'unit_ids':ids,'places':selected,'captures':facts},indent=2)+'\n')
    print('Chosen italic label: four kinds in LIVE sight, Firefox1280+390; eight images and server tier facts')
finally:
    if ids:sql("DELETE FROM units WHERE id IN ("+','.join("'"+i+"'" for i in ids)+")")
