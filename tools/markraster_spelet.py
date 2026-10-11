#!/usr/bin/env python3
"""Read-only renderer instrumentation + real acceptance players and web expedition.
Needs /tmp/markraster-spelet-accounts/all.json made with acceptance.sh player.
No HTTP fixtures, injected map data, SQL placement, or rendering/design changes.
"""
import json,time,hashlib,subprocess,urllib.request
from pathlib import Path
from playwright.sync_api import sync_playwright
ROOT=Path(__file__).resolve().parents[1];OUT=ROOT/'docs/reviews/markraster-spelet';OUT.mkdir(parents=True,exist_ok=True)
BASE='http://localhost:8097';accounts=json.loads(Path('/tmp/markraster-spelet-accounts/all.json').read_text());world=accounts[0]['world_id'];reports=json.loads((OUT/'browser.json').read_text())['reports'] if (OUT/'browser.json').exists() else []
def api(c,tail):
 req=urllib.request.Request(BASE+'/api/v1/worlds/'+world+'/'+tail,headers={'Authorization':'Bearer '+c['token']})
 return json.load(urllib.request.urlopen(req,timeout=20))
source=(ROOT/'web/static/js/megaron/render/map.js').read_text();served=urllib.request.urlopen(BASE+'/static/js/megaron/render/map.js').read();assert served==source.encode(),'mounted renderer differs from worktree'
source=source.replace('export function render() {','function observedRender() {')+'''
export function render() { const t=performance.now();try{return observedRender();}
finally{(window.__blocks ||= []).push(performance.now()-t);if(window.__blocks.length>3000)window.__blocks.shift();} }
'''
queue=(ROOT/'web/static/js/megaron/render/chunk_worker.js').read_text().replace('onChunk(job, data.chunk, data.bakeMs);','(window.__bakes ||= []).push({key:job.key,sig:job.sig,bakeMs:data.bakeMs}); onChunk(job, data.chunk, data.bakeMs);')
setup='''async()=>{window.State=(await import('/static/js/megaron/state.js')).State;window.map=await import('/static/js/megaron/render/map.js');}'''
settle='''async()=>{for(let n=0;n<1200;n++){if(map.markrasterStats.pending===0 && map.markrasterStats.chunks>0)break;await new Promise(r=>setTimeout(r,16));}return {...map.markrasterStats};}'''
refresh='''async()=>{window.__oldTiles=State.tileData;map.refreshTiles();for(let n=0;n<1000;n++){if(State.tileData!==window.__oldTiles && map.markrasterStats.pending===0)break;await new Promise(r=>setTimeout(r,16));}return State.tileData.map(t=>({q:t.q,r:t.r,terrain:t.terrain,tier:t.tier,river_name:t.river_name}));}'''
snapshot='''()=>({stats:{...map.markrasterStats},tiles:State.tileData.map(t=>({q:t.q,r:t.r,terrain:t.terrain,tier:t.tier,river_name:t.river_name})),bakes:window.__bakes||[],blocks:window.__blocks||[],frameGaps:window.__frameGaps||[]})'''
def counts(tiles):return {tier:sum(t['tier']==tier for t in tiles) for tier in ['live','remembered','fog']}
def shot(page,name):page.screenshot(path=str(OUT/(name+'.png')))
with sync_playwright() as pw:
 browser=pw.firefox.launch();metadata={'health':json.load(urllib.request.urlopen(BASE+'/healthz')),'world':world,'browser':browser.version,'asset_sha256':hashlib.sha256(served).hexdigest(),'reports':reports}
 for c in [accounts[0],accounts[2]]:
  for width,height in [(1280,900),(390,844)]:
   if any(r['player']==c['username'] and r['width']==width for r in reports):continue
   context=browser.new_context(viewport={'width':width,'height':height},device_scale_factor=1);context.add_cookies([{'name':'poleia_token','value':c['token'],'url':BASE}]);context.add_init_script('localStorage.setItem("poleia_token",'+json.dumps(c['token'])+');window.__frameGaps=[];let last=performance.now();function probe(t){window.__frameGaps.push(t-last);last=t;if(window.__frameGaps.length>3000)window.__frameGaps.shift();requestAnimationFrame(probe)}requestAnimationFrame(probe);')
   page=context.new_page();errors=[];http_errors=[]
   page.on('pageerror',lambda e:errors.append(str(e)));page.on('console',lambda m:errors.append(m.text) if m.type=='error' else None);page.on('response',lambda r:http_errors.append({'url':r.url,'status':r.status}) if r.status>=400 else None)
   page.route('**/static/js/megaron/render/map.js',lambda r:r.fulfill(content_type='text/javascript',body=source));page.route('**/static/js/megaron/render/chunk_worker.js',lambda r:r.fulfill(content_type='text/javascript',body=queue))
   page.goto(BASE+'/play',wait_until='domcontentloaded');page.evaluate(setup);page.wait_for_function('State.WORLD_ID && State.tileData.length>0 && State.provinceData.length>0',timeout=30000);assert page.evaluate('State.WORLD_ID')==world
   stats=page.evaluate(settle);assert stats['mode']=='worker' and stats['pending']==0,stats
   before=page.evaluate(snapshot);assert counts(before['tiles'])['fog']>0;assert not any(t.get('river_name') for t in before['tiles'] if t['tier']=='fog')
   page.wait_for_timeout(500);shot(page,f'{c["username"]}-{width}-city-before')
   row={'player':c['username'],'width':width,'city':c['founded'],'first_view':{'stats':stats,'max_render_ms':max(before['blocks']),'tiers':counts(before['tiles'])}};reports.append(row)
   # A real equal-content /map refresh should reuse every completed chunk.
   stable_before=page.evaluate(snapshot);same=page.evaluate(refresh);stable_after=page.evaluate(snapshot)
   row['equal_refresh_bakes']=stable_after['stats']['chunks']-stable_before['stats']['chunks'];row['equal_refresh_same_tiles']=same==stable_before['tiles'];
   if row['equal_refresh_same_tiles']:assert row['equal_refresh_bakes']==0,row
   else:
    prior={j['key']:j['sig'] for j in stable_before['bakes']};new=stable_after['bakes'][len(stable_before['bakes']):];assert all(j['key'] not in prior or j['sig']!=prior[j['key']] for j in new),'unchanged signature re-baked on moving viewport'
   if c['username']=='Eirene' and width==1280:
    units=api(c,'units')['units'];chosen=next(u for u in units if u['type']=='spearman' and u['status']=='garrison');uid=chosen['id']
    # Observer SQL selected an actual river six hexes away for this integration
    # check; the player still sees fog there and gives a normal explore order.
    target={'q':6,'r':80}
    page.evaluate('''async target=>{window.menu=await import('/static/js/megaron/ui/marchctx.js');await menu.openMarchCtx({...target,known:false,isSea:false,name:'Unexplored land'},160,200);}''',target)
    index=page.evaluate('''id=>State.marchCtxGroups.findIndex(g=>g.ids.includes(id))''',uid);assert index>=0
    page.locator('#mg-'+str(index)).fill('1');page.locator('#mctx-more summary').click();page.locator('#mctx-ticks').fill('28');shot(page,'Eirene-1280-web-explore-order')
    with page.expect_response(lambda r:'/units/'+uid+'/march' in r.url and r.request.method=='POST') as receipt:page.locator('#mctx-send').click()
    assert receipt.value.status==202,(receipt.value.status,receipt.value.text());row['order_receipt']=receipt.value.json();page.evaluate('menu.closeMarchCtx()')
    old={(t['q'],t['r']):t for t in stable_after['tiles']};history=[];deadline=time.monotonic()+210;seen_name=False;seen_memory=False;seen_live=False
    while time.monotonic()<deadline:
     page.wait_for_timeout(3000);snap_before=page.evaluate(snapshot);tiles=page.evaluate(refresh);snap_after=page.evaluate(snapshot)
     changes=[{'q':t['q'],'r':t['r'],'from':old[(t['q'],t['r'])]['tier'],'to':t['tier']} for t in tiles if t['tier']!=old[(t['q'],t['r'])]['tier']]
     prior={j['key']:j['sig'] for j in snap_before['bakes']};new=snap_after['bakes'][len(snap_before['bakes']):];assert all(j['key'] not in prior or j['sig']!=prior[j['key']] for j in new),'unchanged chunk signature re-baked'
     named=[t for t in tiles if t.get('river_name')];assert not any(t.get('river_name') for t in tiles if t['tier']=='fog')
     seen_live |= any(t['from']=='fog' and t['to']=='live' for t in changes);seen_memory |= any(t['from']=='live' and t['to']=='remembered' for t in changes);seen_name |= bool(named)
     history.append({'tiers':counts(tiles),'changes':changes,'baked_keys':[j['key'] for j in new],'river_tiles':named});old={(t['q'],t['r']):t for t in tiles}
     if seen_live and seen_memory and seen_name:break
    row['history']=history;row['fog_to_live']=seen_live;row['live_to_memory']=seen_memory;row['river_seen']=seen_name
    shot(page,'Eirene-1280-city-after-movement')
    if seen_name:
     river=next(t for t in tiles if t.get('river_name'));page.evaluate('''t=>{const p=map.hexPx(t.q,t.r);State.camera={x:300-p.x*2,y:320-p.y*2,zoom:1};State.dirty=true;}''',river);page.evaluate(settle);page.wait_for_timeout(100);shot(page,'Eirene-1280-river-after-movement')
     rect=page.locator('#hex-canvas').bounding_box();page.mouse.click(rect['x']+300,rect['y']+320);shot(page,'Eirene-1280-river-panel');row['river_panel']={'title':page.locator('#ip-name').inner_text(),'terrain':page.locator('#ip-terrain').inner_text()};page.evaluate('map.closeInspect()')
    (OUT/'movement-tiles.json').write_text('[\n'+',\n'.join(json.dumps(t,separators=(',',':')) for t in tiles)+'\n]\n')
   if c['username']=='Eirene' and width==390:
    river=next((t for t in stable_after['tiles'] if t.get('river_name')),None)
    if river:
     page.evaluate('''t=>{const p=map.hexPx(t.q,t.r);State.camera={x:130-p.x*2,y:300-p.y*2,zoom:1};State.dirty=true;}''',river);page.evaluate(settle);page.wait_for_timeout(100);shot(page,'Eirene-390-river-after-movement')
     rect=page.locator('#hex-canvas').bounding_box();page.mouse.click(rect['x']+130,rect['y']+300);shot(page,'Eirene-390-river-panel');row['river_panel']={'title':page.locator('#ip-name').inner_text(),'terrain':page.locator('#ip-terrain').inner_text()};page.evaluate('map.closeInspect()')
   final=page.evaluate(snapshot);active=sorted(t for t in final['blocks'] if t>0.1);gaps=sorted(final['frameGaps']);row['render']={'active_median_ms':active[len(active)//2] if active else 0,'active_p95_ms':active[int((len(active)-1)*.95)] if active else 0,'max_ms':max(active) if active else 0,'raf_p95_ms':gaps[int((len(gaps)-1)*.95)] if gaps else 0,'stats':final['stats']};row['errors']=errors;row['http_errors']=http_errors
   context.close();(OUT/'browser.json').write_text(json.dumps(metadata,indent=2)+'\n');print(c['username'],width,'tiers',row['first_view']['tiers'],'render',row['render'],'errors',errors,flush=True)
 browser.close()
