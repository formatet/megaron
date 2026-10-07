#!/usr/bin/env python3
"""Compare old/new production renderer on the SAME real scout map snapshot.
Usage: python3 tools/memory_gray_replay.py PROOF_DIR BASELINE_WEB OUT
Complements real web journeys; no server writes, no fabricated visibility.
"""
import json,re,sys
from pathlib import Path
from urllib.parse import urlparse
from playwright.sync_api import sync_playwright
root=Path(__file__).resolve().parents[1];proofdir=Path(sys.argv[1]);baseline=Path(sys.argv[2]);out=Path(sys.argv[3]);out.mkdir(parents=True,exist_ok=True)
proof=json.loads((proofdir/'proof.json').read_text())
world=re.search(r'/worlds/([0-9a-f-]{36})/',(proofdir/'server.log').read_text())[1]
fixture={'world_id':world,'player_id':None,'tiles':proof['tiles'],'provinces':[],'units':[],'rural':[],'marches':[],'messengers':[],'trades':[]}
# Supplementary collision matrix, explicitly synthetic renderer data only.
# Real player journeys and their tiers remain the primary acceptance evidence.
if len(sys.argv)>4 and sys.argv[4]=='collision':
    terrains=['deep_sea','coastal_sea','plains','forest_olive_grove','forest_cedar','hills','mountain_limestone','mountain_red','river','river_valley','river_delta','scrub_maquis','semi_desert','plains']
    matrix=[]
    for row in range(7):
        for q in range(6):
            tier='remembered' if q<2 else 'live' if q<4 else 'fog'
            matrix.append({'q':q,'r':row-q//2,'terrain':terrains[row*2+q%2] if tier!='fog' else 'fog','tier':tier})
    fixture['tiles']=matrix
    # Empty labels isolate painted buildings; player UI deliberately stays coloured.
    fixture['provinces']=[{'q':0,'r':1,'name':'','own':False,'culture':'akhaier','walls':1,'size_tier':0,'army_total':0}, {'q':3,'r':0,'name':'','own':True,'culture':'akhaier','walls':1,'size_tier':0,'army_total':0}]
    fixture['units']=[{'id':'own','type':'spearman','category':'land','status':'positioned','q':3,'r':3,'size':100,'deployable':True}]
    proof['center']={'q':2,'r':2}
results={}
with sync_playwright() as p:
    browser=p.chromium.launch()
    for label,width,height,zoom in [('desktop',1280,900,1),('mobile',390,844,.75),('minimum',1280,900,.3)]:
        before=None
        for arm,web in [('before',baseline),('after',root/'web')]:
            page=browser.new_page(viewport={'width':width,'height':height})
            errors=[];page.on('pageerror',lambda e:errors.append(str(e)))
            def route(req):
                path=urlparse(req.request.url).path
                if path=='/static/fixtures/world-fow.json':req.fulfill(json=fixture);return
                file=web/path.lstrip('/')
                if file.is_file():req.fulfill(path=str(file));return
                req.abort()
            page.route('http://gray-proof.test/**',route)
            page.goto('http://gray-proof.test/static/showcase-world.html?fixture=fow',wait_until='networkidle')
            page.wait_for_function('window.SHOWCASE?.ready')
            page.evaluate('''async ([t,z])=>{
              const {State}=await import('/static/js/megaron/state.js');const {hexPx,canvas,SCALE}=await import('/static/js/megaron/render/map.js');
              document.getElementById('map-root').style.width=innerWidth+'px';document.getElementById('map-root').style.height=innerHeight+'px';dispatchEvent(new Event('resize'));
              const p=hexPx(t.q,t.r);State.camera.zoom=z;State.camera.x=canvas.width/2-p.x*SCALE*z;State.camera.y=canvas.height/2-p.y*SCALE*z;SHOWCASE.draw(65);
            }''',[proof['center'],zoom])
            page.locator('#hex-canvas').screenshot(path=str(out/(arm+'-'+label+'.png')))
            if arm=='before':before=page.locator('#hex-canvas').evaluate("c=>c.toDataURL()");page.close();continue
            metrics=page.evaluate('''async before=>{
              const {State}=await import('/static/js/megaron/state.js');const {hexPx,canvas,SCALE}=await import('/static/js/megaron/render/map.js');const {pixelSpans}=await import('/static/js/megaron/render/memory.js');
              const img=new Image();img.src=before;await img.decode();const old=document.createElement('canvas');old.width=canvas.width;old.height=canvas.height;old.getContext('2d').drawImage(img,0,0);
              const a=old.getContext('2d').getImageData(0,0,old.width,old.height).data,b=canvas.getContext('2d').getImageData(0,0,canvas.width,canvas.height).data;
              const mask=new Uint8Array(canvas.width*canvas.height),k=State.camera.zoom*SCALE,ctx=canvas.getContext('2d');
              ctx.save();ctx.translate(State.camera.x,State.camera.y);ctx.scale(k,k);const m=ctx.getTransform();ctx.restore();
              for(const t of State.tileData.filter(t=>t.tier==='remembered'&&t.terrain!=='fog')){
                const p=hexPx(t.q,t.r),pts=[];for(let i=0;i<6;i++){const angle=Math.PI/3*i;pts.push([m.e+Math.round(p.x+22*Math.cos(angle))*m.a,m.f+Math.round(p.y+22*Math.sin(angle))*m.d]);}
                for(const [x,y,w]of pixelSpans(pts,canvas.width,canvas.height))mask.fill(1,y*canvas.width+x,y*canvas.width+x+w);
              }
              let changed=0,outside=0,coloredMemory=0,maxLightError=0,maxOpaqueLightError=0;const levels=new Set(),anomalies=[];
              for(let n=0;n<mask.length;n++){const i=n*4,diff=a.slice(i,i+4).some((v,j)=>v!==b[i+j]);if(diff){changed++;if(!mask[n])outside++;}
                if(mask[n]){if(Math.max(...b.slice(i,i+3))-Math.min(...b.slice(i,i+3))>1)coloredMemory++;levels.add(b[i]);const error=Math.abs(b[i]-(.3*a[i]+.59*a[i+1]+.11*a[i+2]));maxLightError=Math.max(maxLightError,error);if(a[i+3]===255)maxOpaqueLightError=Math.max(maxOpaqueLightError,error);if((error>1||Math.max(...b.slice(i,i+3))-Math.min(...b.slice(i,i+3))>1)&&anomalies.length<20)anomalies.push({x:n%canvas.width,y:Math.floor(n/canvas.width),before:[...a.slice(i,i+4)],after:[...b.slice(i,i+4)],error});}
              }
              const frozen=canvas.toDataURL();SHOWCASE.draw(65);const deterministic=frozen===canvas.toDataURL();
              return {anomalies,changedPixels:changed,changedOutsideMemory:outside,coloredMemoryPixels:coloredMemory,maxLightError,maxOpaqueLightError,grayLevels:levels.size,deterministic,timing:SHOWCASE.time(30,5)};
            }''',before)
            assert not errors,errors
            assert metrics['changedPixels']>0 and metrics['changedOutsideMemory']==0,metrics
            assert metrics['coloredMemoryPixels']==0 and metrics['maxOpaqueLightError']<=1,metrics
            assert metrics['deterministic'] and metrics['grayLevels']>2,metrics
            results[label]=metrics;page.close()
    browser.close()
(out/'pixel-diff.json').write_text(json.dumps(results,indent=2)+'\n');print(json.dumps(results))
