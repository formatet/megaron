#!/usr/bin/env python3
"""Native Firefox temple DOM proof, with actual server fixture/API and no payload overrides."""
import argparse,hashlib,json,pathlib,urllib.request
from playwright.sync_api import sync_playwright
from fardvag_fixture import api,credentials,BASE
ROOT=pathlib.Path(__file__).resolve().parents[1];OUT=ROOT/'docs/reviews/offer-en-knapp'
p=argparse.ArgumentParser();p.add_argument('mode',choices=['baseline','after','check']);args=p.parse_args()
s=json.loads((OUT/'scenario.json').read_text());token,_=credentials();health=json.load(urllib.request.urlopen(BASE+'/healthz'))
folder=OUT/args.mode;folder.mkdir(exist_ok=True);facts={'health':health,'fixture':s,'captures':[]}
if args.mode=='after':
 served=urllib.request.urlopen(BASE+'/static/js/megaron/ui/drawers/kult.js').read();assert served==(ROOT/'web/static/js/megaron/ui/drawers/kult.js').read_bytes()
 facts['kult_sha256']=hashlib.sha256(served).hexdigest()
with sync_playwright() as pw:
 browser=pw.firefox.launch()
 for mode,(width,height) in {'desktop':(1280,900),'mobile':(390,844)}.items():
  ctx=browser.new_context(viewport={'width':width,'height':height});ctx.add_cookies([{'name':'poleia_token','value':token,'url':BASE}]);ctx.add_init_script(f"localStorage.setItem('poleia_token',{json.dumps(token)})")
  page=ctx.new_page();errors=[];page.on('pageerror',lambda e:errors.append(str(e)));requests=[]
  page.on('request',lambda req:requests.append(json.loads(req.post_data)) if req.method=='POST' and req.url.endswith('/rite') else None)
  page.goto(BASE+'/world/'+s['world']+'/map',wait_until='networkidle');page.evaluate("openDrawer('kult')")
  page.wait_for_function("document.getElementById('kult-body')?.textContent.includes('Prayers')")
  page.evaluate("async()=>await Promise.all([...document.querySelectorAll('.drawer.open')].flatMap(d=>d.getAnimations()).map(a=>a.finished.catch(()=>{})))")
  composer=page.locator('.offer-composer').count();numbers=page.locator('#kult-body input[type=number]').count()
  if args.mode=='baseline':assert composer==3 and numbers>0,(composer,numbers)
  else:
   assert composer==0 and numbers==0,'composed offering leaked into DOM'
   assert 'Compose your own offering' not in page.locator('#kult-body').inner_text()
   catalog=api(f'worlds/{s["world"]}/provinces/{s["province"]}',token)['settlement']['available_prayers']
   expected=sum(1 for p in catalog if p['affordable'] and not p.get('cooldown_remaining_minutes',0))
   labels=page.locator('#kult-body button[onclick^="okRite("]').all_text_contents()
   assert expected>0 and labels==['Offer']*expected,(labels,expected)
  if args.mode!='check':page.screenshot(path=str(folder/(mode+'.png')))
  facts['captures'].append({'mode':mode,'composer_count':composer,'numeric_count':numbers,'errors':list(errors),'prayer_ids':page.locator('#kult-body button[onclick^="okRite("]').evaluate_all('(els)=>els.map(e=>e.getAttribute("onclick"))')})
  if args.mode=='after' and mode=='desktop':
   prayer=next(x for x in s['prayer_ids'] if 'oracle' in x)
   before=api(f'worlds/{s["world"]}/provinces/{s["province"]}',token)
   button=page.locator(f'button[onclick="okRite(\'{prayer}\')"]')
   with page.expect_response(lambda r:r.request.method=='POST' and r.url.endswith('/rite')) as reply:button.click()
   assert reply.value.status==200,(reply.value.status,reply.value.text())
   page.locator('#kult-body [data-inline-result]').wait_for()
   assert requests==[{'prayer':prayer}],requests
   facts['action']={'request':requests[0],'status':reply.value.status,'inline':page.locator('#kult-body [data-inline-result]').inner_text(),'before':before,'after':api(f'worlds/{s["world"]}/provinces/{s["province"]}',token),'sql_writes_during_action':False}
   page.screenshot(path=str(folder/'desktop-result.png'))
  ctx.close()
 browser.close()
assert all(not c['errors'] for c in facts['captures']),facts
if args.mode!='check':(folder/'facts.json').write_text(json.dumps(facts,indent=2)+'\n')
print(args.mode,health,'native DOM and Firefox1280+390 green')
