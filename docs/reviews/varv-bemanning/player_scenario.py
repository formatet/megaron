#!/usr/bin/env python3
"""Actual HTTP+CLI+Firefox against fresh isolated handler/PG rig. No prod."""
import json,sys,os,tempfile,subprocess,re,urllib.request
from pathlib import Path
from playwright.sync_api import sync_playwright,expect
c=json.load(sys.stdin);out=Path(__file__).resolve().parent
base=c['base']+'/api/v1/worlds/'+c['world'];city=base+'/provinces/'+c['province']
def request(url,body=None):
 req=urllib.request.Request(url,headers={'Authorization':'Bearer '+c['token'],'Content-Type':'application/json'},data=None if body is None else json.dumps(body).encode())
 return json.load(urllib.request.urlopen(req))
with tempfile.TemporaryDirectory() as tmp:
 config=Path(tmp)/'config.json';config.write_text(json.dumps({'server':c['base'],'token':c['token'],'world_id':c['world'],'province_id':c['province'],'player_id':c['player']}));config.chmod(0o600)
 env={k:v for k,v in os.environ.items() if not k.startswith('POLEIA_')};env['POLEIA_CONFIG']=str(config)
 transcript=[]
 def cli(*args):
  p=subprocess.run([c['cli'],*args],env=env,capture_output=True,text=True)
  assert p.returncode==0,(args,p.stdout,p.stderr)
  transcript.append('$ keryx '+' '.join(args)+'\n'+p.stdout);return p.stdout
 opts=request(city+'/placement-options');yard=next(b for b in opts['buildings'] if b['building_type']=='shipyard');assert yard['goods'][0]['cap']==3 and yard['goods'][0]['placed']==0
 build=cli('recruit','--unit','galley');assert 'waiting for shipyard workers' in build.lower(),build
 repair=request(base+'/units/'+c['repair_ship']+'/repair',{})
 assert repair['shipyard_waiting'] and repair['complete_at'] is None,repair
 paused=request(base+'/units');assert len(paused['units'])==2
 assert all(u['shipyard_waiting'] and not u.get('build_complete_at') for u in paused['units'])
 assert 'waiting for shipyard workers' in cli('unit','list')
 (out/f'api-paused-run{c["run"]}.json').write_text(json.dumps(paused,indent=2)+'\n')
 # Render real cards with actual endpoint responses, as in the established freight proof.
 with sync_playwright() as pw:
  browser=pw.firefox.launch();context=browser.new_context(viewport={'width':390,'height':844},device_scale_factor=1,has_touch=True);page=context.new_page();errors=[];page.on('pageerror',lambda e:errors.append(str(e)))
  html=(out.parents[2]/'web/static/map.html').read_text();html=re.sub(r'<script\b[^>]*>.*?</script>','',html,flags=re.S)
  page.route('**/fixture',lambda r:r.fulfill(content_type='text/html',body=html))
  main=(out.parents[2]/'web/static/js/megaron/main.js').read_text().split('(async function start() {')[0]
  page.route('**/static/js/megaron/main.js',lambda r:r.fulfill(content_type='application/javascript',body=main))
  page.add_init_script('window.requestAnimationFrame=()=>0;window.setInterval=()=>0;')
  page.goto(c['base']+'/fixture');page.evaluate('''async c=>{localStorage.setItem('poleia_token',c.token);await import('/static/js/megaron/main.js');const {State}=await import('/static/js/megaron/state.js');State.WORLD_ID=c.world;State.MY_PLAYER_ID=c.player;State.MY_SETTLEMENT_ID=c.city;State.provinceData=[{id:c.province,settlement_id:c.city,own:true,is_capital:true,q:0,r:0,name:'Drydock'}];State.tileData=[];}''',c)
  page.locator('#trig-war').tap();body=page.locator('#wtab-army');expect(body).to_contain_text('waiting for shipyard workers')
  for width in [390,1000]:
   page.set_viewport_size({'width':width,'height':844});page.screenshot(path=str(out/f'firefox-{width}-run{c["run"]}.png'))
  page.locator('#trig-city').tap()
  page.locator('.gubbe-hex-g[data-target="city"]').tap()
  row=page.locator('[data-good="shipbuilding"]');expect(row).to_be_visible()
  expect(row).to_contain_text('waiting for shipyard workers')
  row.get_by_role('button',name='+1',exact=True).tap()
  expect(row).to_contain_text('full working pace')
  for width in [390,1000]:
   page.set_viewport_size({'width':width,'height':844});page.screenshot(path=str(out/f'firefox-city-{width}-run{c["run"]}.png'))
  row.get_by_role('button',name='−1',exact=True).tap()
  expect(row).to_contain_text('waiting for shipyard workers')
  assert not errors,errors;browser.close()
 cli('staff','shipyard','+1');running=request(base+'/units');assert all(not u['shipyard_waiting'] and u.get('build_complete_at') for u in running['units'])
 (out/f'api-running-run{c["run"]}.json').write_text(json.dumps(running,indent=2)+'\n')
 cli('staff','shipyard','-1');paused_again=request(base+'/units');assert all(u['shipyard_waiting'] and not u.get('build_complete_at') for u in paused_again['units'])
 cli('staff','shipyard','+1');assert 'waiting for shipyard workers' not in cli('unit','list')
 (out/f'cli-run{c["run"]}.txt').write_text('\n'.join(transcript))
print(json.dumps({'run':c['run'],'actual_http':True,'actual_cli':True,'firefox':[390,1000],'jobs':2,'pause_resume':True,'page_errors':errors}))
