#!/usr/bin/env python3
"""Actual read-only status on authenticated local handlers and synthetic cities."""
import json,sys,os,tempfile,subprocess,re,urllib.request
from pathlib import Path
from playwright.sync_api import sync_playwright,expect
c=json.load(sys.stdin);out=Path(__file__).resolve().parent
with tempfile.TemporaryDirectory(prefix='megaron-order25-cli-') as tmp:
 config=Path(tmp)/'config.json';config.write_text(json.dumps({'server':c['base'],'token':c['token'],'world_id':c['world'],'province_id':c['province'],'player_id':c['player']}))
 env={k:v for k,v in os.environ.items() if not k.startswith('POLEIA_')};env['POLEIA_CONFIG']=str(config)
 p=subprocess.run([c['cli'],'status'],env=env,capture_output=True,text=True)
 assert p.returncode==0,p.stderr
 for text in ['Everyday prayers work at a level-1 temple','at least 60 kharis','with or without a temple','Protected from divine punishment until']:
  assert text in p.stdout,(text,p.stdout)
 (out/f"cli-run{c['run']}.txt").write_text(p.stdout)
 n=subprocess.run([c['cli'],'notifications','--kind','KharisDeclining'],env=env,capture_output=True,text=True)
 assert n.returncode==0 and 'Kharis is falling' in n.stdout and '23 days' in n.stdout,(n.stdout,n.stderr)
 (out/f"cli-notice-run{c['run']}.txt").write_text(n.stdout)
 req=urllib.request.Request(c['base']+'/api/v1/worlds/'+c['world']+'/provinces/'+c['province'],headers={'Authorization':'Bearer '+c['token']})
 data=json.load(urllib.request.urlopen(req));s=data['settlement']
 assert s['kharis_wrath']['protection_remaining_days']==7 and s['kharis_wrath']['protection_until']
 (out/f"api-run{c['run']}.json").write_text(json.dumps(data,indent=2)+'\n')
 req=urllib.request.Request(c['base']+'/api/v1/worlds/'+c['world']+'/notifications?kind=KharisDeclining',headers={'Authorization':'Bearer '+c['token']})
 notice=json.load(urllib.request.urlopen(req))['notifications'][0]
 c['warning_body']=notice['body']
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
  for text in ['Everyday prayers work at a level-1 temple','at least 60 kharis','with or without a temple','Protected from divine punishment','until']:
   expect(body).to_contain_text(text)
  for width in [390,1000]:
   page.set_viewport_size({'width':width,'height':844});body.evaluate('el=>el.scrollTop=0')
   page.screenshot(path=str(out/f"firefox-{width}-run{c['run']}.png"))
  page.evaluate('''async c=>{const {openDispatchWindow}=await import('/static/js/megaron/ui/dispatch_window.js');openDispatchWindow('KharisDeclining',c.warning_body,'now');}''',c)
  expect(page.locator('#dw-body')).to_contain_text('Kharis is falling')
  expect(page.locator('#dw-body')).to_contain_text('23 days')
  page.screenshot(path=str(out/f"firefox-warning-run{c['run']}.png"))
  assert not errors,errors
  browser.close()
 print(json.dumps({'run':c['run'],'cli_status':True,'cli_warning':True,'api_shield':True,'browser_status':True,'browser_actual_warning':True,'page_errors':errors}))
