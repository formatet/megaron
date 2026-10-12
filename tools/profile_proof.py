#!/usr/bin/env python3
"""Own-world profile through real API, CLI and native Firefox; no payload overrides."""
import hashlib,json,os,pathlib,subprocess,tempfile,urllib.request
from playwright.sync_api import sync_playwright
from fardvag_fixture import api,BASE,PW
ROOT=pathlib.Path(__file__).resolve().parents[1]
OUT=ROOT/'docs/reviews/spelarprofil';OUT.mkdir(exist_ok=True)
WORLD='3b5fb9cd-8c06-4685-88df-b162c9acff99'
facts={'world':WORLD,'health':json.load(urllib.request.urlopen(BASE+'/healthz')),'sql_writes':False,'payload_overrides':False,'captures':[],'assets':{}}
for rel in ['js/megaron/ui/account_window.js','js/megaron/ui/account_profile.js','megaron.css','codex/account.md']:
 data=urllib.request.urlopen(BASE+'/static/'+rel).read()
 assert data==(ROOT/'web/static'/rel).read_bytes(),rel
 facts['assets'][rel]=hashlib.sha256(data).hexdigest()
with tempfile.TemporaryDirectory(prefix='profile-proof-') as tmp:
 tmp=pathlib.Path(tmp)
 subprocess.run(['go','build','-o',str(tmp/'keryx'),'./cmd/keryx'],cwd=ROOT/'server',check=True)
 with sync_playwright() as pw:
  browser=pw.firefox.launch()
  for username,kind in [('Eirene','founded'),('ParityNomad','host')]:
   token=api('auth/login',data={'username_or_email':username,'password':PW})['access_token']
   profile=api(f'worlds/{WORLD}/me',token)
   assert set(profile)=={'wanax_name','founded_tick','cities','population','kharis'},profile
   if kind=='host':assert profile['founded_tick'] is None and profile['cities']==0 and profile['population']==0,profile
   else:assert profile['founded_tick']==9 and profile['cities']>0,profile
   config=tmp/'config.json';config.write_text(json.dumps({'server':BASE,'token':token,'world_id':WORLD}));config.chmod(0o600)
   env=dict(os.environ,POLEIA_CONFIG=str(config));env.pop('POLEIA_SERVER',None);env.pop('POLEIA_TOKEN',None)
   text=subprocess.check_output([str(tmp/'keryx'),'me'],env=env,text=True)
   raw=json.loads(subprocess.check_output([str(tmp/'keryx'),'me','--json'],env=env,text=True))
   assert len(text.strip().splitlines())==5 and raw['wanax_name']==profile['wanax_name']
   (OUT/(kind+'-cli.txt')).write_text(text)
   facts[kind]={'api':profile,'cli_json':raw}
   for label,width,height in [('desktop',1280,900),('mobile',390,844)]:
    context=browser.new_context(viewport={'width':width,'height':height})
    context.add_cookies([{'name':'poleia_token','value':token,'url':BASE}])
    context.add_init_script(f"localStorage.setItem('poleia_token',{json.dumps(token)})")
    page=context.new_page();errors=[];page.on('pageerror',lambda e:errors.append(str(e)))
    page.goto(BASE+'/world/'+WORLD+'/map',wait_until='networkidle')
    with page.expect_response(lambda r:r.url.endswith('/worlds/'+WORLD+'/me')) as reply:
     page.locator('#gt-wanax').click()
    assert reply.value.status==200
    received=reply.value.json()
    page.wait_for_function("document.querySelectorAll('#acc-profile .stat-row').length===5")
    rows=page.locator('#acc-profile .stat-row').all_text_contents()
    assert received['wanax_name'] in rows[0],rows
    assert ('Not yet founded' if received['founded_tick'] is None else 'Since day '+str(received['founded_tick'])) in rows[1],rows
    assert page.locator('#acc-profile').evaluate('(el)=>el.getBoundingClientRect().right<=innerWidth && el.getBoundingClientRect().left>=0')
    assert page.locator('#account-window-overlay .dispatch-window').evaluate('(el)=>el.getBoundingClientRect().bottom<=innerHeight+1')
    page.screenshot(path=str(OUT/(kind+'-'+label+'.png')))
    facts['captures'].append({'kind':kind,'viewport':[width,height],'rows':rows,'api':received,'errors':list(errors),'url':page.url})
    context.close()
  browser.close()
assert all(not c['errors'] for c in facts['captures']),facts['captures']
(OUT/'verification.json').write_text(json.dumps(facts,indent=2)+'\n')
print('own API/CLI; founded + Host Firefox1280/390; five rows; no SQL writes or overrides; green')
