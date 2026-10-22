#!/usr/bin/env python3
"""Actual read-only status on authenticated local handlers and synthetic cities."""
import json,sys,os,tempfile,subprocess,re,urllib.request
from pathlib import Path
from playwright.sync_api import sync_playwright,expect
c=json.load(sys.stdin);out=Path(__file__).resolve().parent
with tempfile.TemporaryDirectory(prefix='megaron-order24-cli-') as tmp:
 config=Path(tmp)/'config.json';config.write_text(json.dumps({'server':c['base'],'token':c['token'],'world_id':c['world'],'province_id':c['province'],'player_id':c['player']}))
 env={k:v for k,v in os.environ.items() if not k.startswith('POLEIA_')};env['POLEIA_CONFIG']=str(config)
 p=subprocess.run([c['cli'],'status'],env=env,capture_output=True,text=True)
 assert p.returncode==0,p.stderr
 for text in ['Upkeep of the cult: 0.10 base + 2 cities cost 0.20 + 3500 residents cost 0.35 = 0.65','Temple production: 2.10','net +1.5','worker gives +1.05','before scarcity']:
  assert text in p.stdout,(text,p.stdout)
 (out/f"cli-run{c['run']}.txt").write_text(p.stdout)
 req=urllib.request.Request(c['base']+'/api/v1/worlds/'+c['world']+'/provinces/'+c['province'],headers={'Authorization':'Bearer '+c['token']})
 data=json.load(urllib.request.urlopen(req));s=data['settlement']
 assert s['kharis_population']==3500 and abs(s['kharis_population_decay']-.35)<1e-9
 assert abs(s['kharis_daily_decay']-.65)<1e-9 and abs(s['kharis_production_per_tick']-2.1)<1e-9 and abs(s['kharis_net_per_tick']-1.45)<1e-9
 (out/f"api-run{c['run']}.json").write_text(json.dumps(data,indent=2)+'\n')
 with sync_playwright() as pw:
  browser=pw.firefox.launch();page=browser.new_page(viewport={'width':390,'height':844},device_scale_factor=1)
  errors=[];page.on('pageerror',lambda e:errors.append(str(e)))
  html=(out.parents[2]/'web/static/map.html').read_text();html=re.sub(r'<script\b[^>]*>.*?</script>','',html,flags=re.S)
  page.route('**/fixture',lambda r:r.fulfill(content_type='text/html',body=html))
  main=(out.parents[2]/'web/static/js/megaron/main.js').read_text().split('(async function start() {')[0]
  page.route('**/static/js/megaron/main.js',lambda r:r.fulfill(content_type='application/javascript',body=main))
  page.add_init_script("window.requestAnimationFrame=()=>0;window.setInterval=()=>0;")
  page.goto(c['base']+'/fixture')
  page.evaluate('''async c=>{
   localStorage.setItem('poleia_token',c.token);await import('/static/js/megaron/main.js');
   const {State}=await import('/static/js/megaron/state.js');
   State.WORLD_ID=c.world;State.MY_PLAYER_ID=c.player;State.MY_SETTLEMENT_ID=c.city;
   State.provinceData=[{id:c.province,settlement_id:c.city,own:true,is_capital:true,q:0,r:0,name:'Alpha City'}];
   State.tileData=[];State.CURRENT_TICK=100;State.TICK_SECONDS=60;State.TICK_ANCHOR_MS=Date.now();
  }''',c)
  page.locator('#trig-kult').click()
  body=page.locator('#kult-body')
  for text in ['Temple production','2.10 kharis per day','Upkeep of the cult','3500 residents cost 0.35','0.65 kharis per day','worker gives +1.05','before scarcity','Requires a free citizen']:
   expect(body).to_contain_text(text)
  for width in [390,1000]:
   page.set_viewport_size({'width':width,'height':844});body.evaluate('el=>el.scrollTop=0')
   page.screenshot(path=str(out/f"firefox-{width}-run{c['run']}.png"))
  assert not errors,errors
  browser.close()
 print(json.dumps({'run':c['run'],'cli_status':True,'api_breakdown':True,'browser_status':True,'page_errors':errors}))
