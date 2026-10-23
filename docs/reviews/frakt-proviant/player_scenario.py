#!/usr/bin/env python3
import json,sys,os,tempfile,subprocess,re,urllib.request
from pathlib import Path
from playwright.sync_api import sync_playwright,expect
c=json.load(sys.stdin);out=Path(__file__).resolve().parent
req=urllib.request.Request(c['base']+'/api/v1/worlds/'+c['world']+'/units',headers={'Authorization':'Bearer '+c['token']})
data=json.load(urllib.request.urlopen(req));units=data['units'];assert len(units)==3
(out/f"api-run{c['run']}.json").write_text(json.dumps(data,indent=2)+'\n')
with tempfile.TemporaryDirectory() as tmp:
 config=Path(tmp)/'config.json';config.write_text(json.dumps({'server':c['base'],'token':c['token'],'world_id':c['world'],'province_id':c['province'],'player_id':c['player']}))
 env={k:v for k,v in os.environ.items() if not k.startswith('POLEIA_')};env['POLEIA_CONFIG']=str(config)
 p=subprocess.run([c['cli'],'unit','list'],env=env,capture_output=True,text=True)
 assert p.returncode==0,(p.stdout,p.stderr)
 for text in ['food for 7 days','food for less than a day LOW','OUT OF FOOD','0.75 fish']:assert text in p.stdout,(text,p.stdout)
 (out/f"cli-run{c['run']}.txt").write_text(p.stdout)
with sync_playwright() as pw:
 browser=pw.firefox.launch();context=browser.new_context(viewport={'width':390,'height':844},device_scale_factor=1,has_touch=True);page=context.new_page();errors=[];page.on('pageerror',lambda e:errors.append(str(e)))
 html=(out.parents[2]/'web/static/map.html').read_text();html=re.sub(r'<script\b[^>]*>.*?</script>','',html,flags=re.S)
 page.route('**/fixture',lambda r:r.fulfill(content_type='text/html',body=html))
 main=(out.parents[2]/'web/static/js/megaron/main.js').read_text().split('(async function start() {')[0]
 page.route('**/static/js/megaron/main.js',lambda r:r.fulfill(content_type='application/javascript',body=main))
 page.add_init_script("window.requestAnimationFrame=()=>0;window.setInterval=()=>0;")
 page.goto(c['base']+'/fixture');page.evaluate('''async c=>{
 localStorage.setItem('poleia_token',c.token);await import('/static/js/megaron/main.js');const {State}=await import('/static/js/megaron/state.js');
 State.WORLD_ID=c.world;State.MY_PLAYER_ID=c.player;State.MY_SETTLEMENT_ID=c.city;
 State.provinceData=[{id:c.province,settlement_id:c.city,own:true,is_capital:true,q:0,r:0,name:'Alpha City'}];State.tileData=[];
 }''',c)
 page.locator('#trig-war').tap();body=page.locator('#wtab-army')
 for text in ['food for 7 days','food for less than a day','out of food','0.8 fish']:expect(body).to_contain_text(text)
 page.locator('#war-body').get_by_role('button',name='Recruit',exact=True).tap()
 page.locator('#war-body').get_by_role('button',name='Army',exact=True).tap()
 expect(body).to_contain_text('food for less than a day')
 for width in [390,1000]:
  page.set_viewport_size({'width':width,'height':844});page.screenshot(path=str(out/f"firefox-{width}-run{c['run']}.png"))
 for u in units:
  card=page.locator('#ucard-'+u['id']);assert card.get_by_role('button',name='Mission',exact=True).count()==0
 assert not errors,errors
 browser.close()
print(json.dumps({'run':c['run'],'actual_api':True,'actual_cli':True,'firefox_gauges':True,'tap_tabs':True,'page_errors':errors}))
