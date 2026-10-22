#!/usr/bin/env python3
"""Actual demolition commands/handlers, seeded two-city realm; no mocked game APIs."""
import json,sys,os,tempfile,subprocess,re,urllib.request
from pathlib import Path
from playwright.sync_api import sync_playwright,expect
c=json.load(sys.stdin);out=Path(__file__).resolve().parent
base=c['base']+'/api/v1/worlds/'+c['world']
def get(path):
 return json.load(urllib.request.urlopen(urllib.request.Request(base+path,headers={'Authorization':'Bearer '+c['token']})))
with tempfile.TemporaryDirectory(prefix='megaron-order26-cli-') as tmp:
 config=Path(tmp)/'config.json';config.write_text(json.dumps({'server':c['base'],'token':c['token'],'world_id':c['world'],'province_id':c['province'],'player_id':c['player']}))
 env={k:v for k,v in os.environ.items() if not k.startswith('POLEIA_')};env['POLEIA_CONFIG']=str(config)
 args=[c['cli'],'demolish','--type','temple','--god','potnia']
 preview=subprocess.run(args,env=env,capture_output=True,text=True)
 assert preview.returncode!=0 and '--yes' in preview.stderr,(preview.stdout,preview.stderr)
 assert get('/provinces/'+c['province'])['settlement']['kharis']==100
 actual=subprocess.run(args+['--yes'],env=env,capture_output=True,text=True)
 assert actual.returncode==0,(actual.stdout,actual.stderr)
 for text in ['30','100','70','50','Potnia','angered']:assert text in actual.stdout,(text,actual.stdout)
 (out/f"cli-run{c['run']}.txt").write_text('UNCONFIRMED\n'+preview.stdout+preview.stderr+'\nCONFIRMED\n'+actual.stdout)
 buildings=get('/provinces/'+c['web_province']+'/buildings');quote=buildings[0]['demolition_kharis']
 assert quote['kharis_before']==70 and quote['kharis_after']==60 and quote['cap_before']==50 and quote['cap_after']==40,quote
 (out/f"quote-run{c['run']}.json").write_text(json.dumps(buildings,indent=2)+'\n')
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
   State.provinceData=[{id:c.province,settlement_id:c.city,own:true,is_capital:true,q:0,r:0,name:'Alpha City'},
    {id:c.web_province,settlement_id:c.web_city,own:true,is_capital:false,q:1,r:0,name:'Alpha Colony'}];
   State.cityViewID=c.web_province;State.tileData=[];State.CURRENT_TICK=100;State.TICK_SECONDS=60;State.TICK_ANCHOR_MS=Date.now();
  }''',c)
  page.locator('#trig-city').click()
  page.get_by_role('button',name='Buildings',exact=True).click()
  sec=page.locator('#city-bld-sec');expect(sec).to_contain_text('Temple of Potnia')
  sec.get_by_role('button',name='Demolish',exact=True).click()
  host=page.locator('#city-demolish-res');expect(host).to_contain_text('70 → 60');expect(host).to_contain_text('50 → 40')
  assert get('/provinces/'+c['web_province'])['settlement']['kharis']==70
  for width in [390,1000]:
   page.set_viewport_size({'width':width,'height':844});host.scroll_into_view_if_needed();page.screenshot(path=str(out/f"firefox-confirm-{width}-run{c['run']}.png"))
  host.get_by_role('button',name='Demolish',exact=True).click()
  expect(sec).to_contain_text('70 → 60');expect(sec).to_contain_text('50 → 40')
  page.screenshot(path=str(out/f"firefox-result-run{c['run']}.png"))
  notices=get('/notifications?kind=TempleDemolitionWrath')['notifications'];assert len(notices)==2,notices
  c['wrath_body']=notices[0]['body'];(out/f"notices-run{c['run']}.json").write_text(json.dumps(notices,indent=2)+'\n')
  page.evaluate('''async c=>{const {openDispatchWindow}=await import('/static/js/megaron/ui/dispatch_window.js');openDispatchWindow('TempleDemolitionWrath',c.wrath_body,'now');}''',c)
  expect(page.locator('#dw-body')).to_contain_text('angered');expect(page.locator('#dw-body')).to_contain_text('Potnia')
  page.screenshot(path=str(out/f"firefox-notice-run{c['run']}.png"))
  assert not errors,errors
  browser.close()
 after=get('/provinces/'+c['web_province']);assert after['settlement']['kharis']==60 and after['settlement']['kharis_cap']==40,after
 (out/f"api-after-run{c['run']}.json").write_text(json.dumps(after,indent=2)+'\n')
 status=subprocess.run([c['cli'],'status'],env=env,capture_output=True,text=True);assert status.returncode==0 and 'offerings do not raise it' in status.stdout,(status.stdout,status.stderr)
 (out/f"cli-status-run{c['run']}.txt").write_text(status.stdout)
 print(json.dumps({'run':c['run'],'unconfirmed_no_delete':True,'cli_actual_delete':True,'browser_actual_delete':True,'persisted_notices':2,'above_ceiling_standing':60,'cap':40,'page_errors':errors}))
