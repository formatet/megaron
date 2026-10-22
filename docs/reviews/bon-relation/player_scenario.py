#!/usr/bin/env python3
"""Actual authenticated HTTP and DB via temporary Go harness; stdin-only secrets."""
import json,sys,os,tempfile,subprocess,re
from pathlib import Path
from playwright.sync_api import sync_playwright,expect
c=json.load(sys.stdin);out=Path(__file__).resolve().parent
with tempfile.TemporaryDirectory(prefix='megaron-order23-cli-') as tmp:
 config=Path(tmp)/'config.json';config.write_text(json.dumps({'server':c['base'],'token':c['token'],'world_id':c['world'],'province_id':c['province'],'player_id':c['player']}))
 env={k:v for k,v in os.environ.items() if not k.startswith('POLEIA_')};env['POLEIA_CONFIG']=str(config)
 def cli(*args):
  p=subprocess.run([c['cli'],'rite',*args],env=env,capture_output=True,text=True)
  assert p.returncode==0,p.stderr
  return p.stdout
 listing=cli('--list');assert 'requirement, never spent' in listing
 # Asterion takes four timber and three silver; independent god from browser's Potnia.
 cast=cli('--prayer','minoan_asterion_thread','--settlement',c['city'],'--yes','--offer-multiplier','2')
 assert "thread waits" in cast and 'Kharis spent' not in cast,cast
 (out/f"cli-run{c['run']}.txt").write_text(listing+'\n'+cast)
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
   localStorage.setItem('poleia_token',c.token);
   await import('/static/js/megaron/main.js');
   const {State}=await import('/static/js/megaron/state.js');
   State.WORLD_ID=c.world;State.MY_PLAYER_ID=c.player;State.MY_SETTLEMENT_ID=c.city;
   State.provinceData=[{id:c.province,settlement_id:c.city,own:true,is_capital:true,q:0,r:0,name:'Alpha City'}];
   State.tileData=[];State.CURRENT_TICK=100;State.TICK_SECONDS=60;State.TICK_ANCHOR_MS=Date.now();
  }''',c)
  page.locator('#trig-kult').click()
  expect(page.locator('#kult-body')).to_contain_text('requirement, not spent')
  expect(page.locator('#kult-body')).to_contain_text('More offering will not improve the answer.')
  button=page.locator('button[onclick="okRite(\'minoan_potnia_walls\')"]');button.scroll_into_view_if_needed();page.screenshot(path=str(out/f"firefox-390-before-run{c['run']}.png"));button.click()
  row=page.locator('#offer-row-minoan_potnia_walls');expect(row).to_contain_text('Bring the offering')
  row.get_by_role('button',name='Bring the offering').click()
  expect(page.locator('#kult-body [data-inline-result]')).to_contain_text('Potnia shelters your walls')
  page.locator('#kult-body [data-inline-result]').scroll_into_view_if_needed()
  page.screenshot(path=str(out/f"firefox-390-after-run{c['run']}.png"))
  page.set_viewport_size({'width':1000,'height':844});page.screenshot(path=str(out/f"firefox-1000-after-run{c['run']}.png"))
  assert not errors,errors
  browser.close()
 print(json.dumps({'run':c['run'],'cli_list':True,'cli_cast':True,'browser_cast':True,'page_errors':errors}))
