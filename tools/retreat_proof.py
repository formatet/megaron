#!/usr/bin/env python3
"""Normal realm retreat web/CLI/API proof, Firefox 1280+390; no SQL writes.
Baseline captures the existing rig process before rebuilding. After verifies
both choices, retired API/CLI requests, and restores the initial doctrine.
"""
import argparse,hashlib,json,os,pathlib,subprocess,tempfile,urllib.error,urllib.request
from playwright.sync_api import sync_playwright
from fardvag_fixture import api,BASE,PW
ROOT=pathlib.Path(__file__).resolve().parents[1]
OUT=ROOT/'docs/reviews/retreat-tva-lagen'
p=argparse.ArgumentParser();p.add_argument('mode',choices=['baseline','after']);args=p.parse_args()
folder=OUT/args.mode;folder.mkdir(parents=True,exist_ok=True)
token=api('auth/login',data={'username_or_email':'ParityNomad','password':PW})['access_token']
for world in api('worlds',token):
 try:initial=api(f'worlds/{world["id"]}/retreat-default',token);break
 except urllib.error.HTTPError as e:
  if e.code!=404:raise
else:raise RuntimeError('ParityNomad has not joined an available world')
wid=world['id'];path=f'worlds/{wid}/retreat-default'
health=json.load(urllib.request.urlopen(BASE+'/healthz'));facts={'health':health,'world':wid,'initial':initial,'no_sql_writes':True,'captures':[]}
if args.mode=='after':
 expected=subprocess.check_output(['git','-C',str(ROOT),'rev-parse','--short=7','HEAD'],text=True).strip()
 assert health['commit']==expected and health['migration']==173,health
 facts['assets']={}
 for rel in ['js/megaron/ui/retreat.js','js/megaron/ui/drawers/war.js']:
  served=urllib.request.urlopen(BASE+'/static/'+rel).read();assert served==(ROOT/'web/static'/rel).read_bytes(),rel
  facts['assets'][rel]=hashlib.sha256(served).hexdigest()
with tempfile.TemporaryDirectory(prefix='retreat-cli-') as tmp:
 tmp=pathlib.Path(tmp);binary=tmp/'keryx'
 if args.mode=='after':subprocess.run(['go','build','-o',str(binary),'./cmd/keryx'],cwd=ROOT/'server',check=True)
 cfg=tmp/'config.json';cfg.write_text(json.dumps({'server':BASE,'token':token,'world_id':wid,'username':'ParityNomad'}));cfg.chmod(0o600)
 env=dict(os.environ,POLEIA_CONFIG=str(cfg));env.pop('POLEIA_SERVER',None);env.pop('POLEIA_TOKEN',None)
 def cli(flags):return subprocess.run([str(binary),'retreat-default',*flags],env=env,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
 try:
  with sync_playwright() as pw:
   browser=pw.firefox.launch()
   for mode,(width,height) in {'desktop':(1280,900),'mobile':(390,844)}.items():
    ctx=browser.new_context(viewport={'width':width,'height':height});ctx.add_cookies([{'name':'poleia_token','value':token,'url':BASE}]);ctx.add_init_script(f"localStorage.setItem('poleia_token',{json.dumps(token)})")
    page=ctx.new_page();errors=[];page.on('pageerror',lambda e:errors.append(str(e)))
    page.goto(BASE+'/world/'+wid+'/map',wait_until='networkidle');page.evaluate("openDrawer('war')")
    page.locator('#war-retreat-default').wait_for()
    page.evaluate("async()=>await Promise.all([...document.querySelectorAll('.drawer.open')].flatMap(d=>d.getAnimations()).map(a=>a.finished.catch(()=>{})))")
    page.wait_for_function("(()=>{const r=document.getElementById('war-retreat-default')?.getBoundingClientRect();return r && r.x>=0 && r.right<=innerWidth;})()")
    choices=['loyalty','hold'] if args.mode=='after' else ['baseline']
    for choice in choices:
     if args.mode=='after':
      page.locator('#war-retreat-default').select_option(choice)
      page.locator('button[onclick="saveRetreatDefault()"]').click()
      page.wait_for_function("document.getElementById('war-retreat-default-res')?.textContent.startsWith('Saved')")
      value=api(path,token);assert value['by_loyalty']==(choice=='loyalty') and value['hold_to_last_man']==(choice=='hold') and value['retreat_at_loss'] is None,value
     else:value=api(path,token)
     options=page.locator('#war-retreat-default option').all_text_contents()
     if args.mode=='after':assert options==['Retreat when it goes badly','Hold to the last man'],options
     box=page.locator('#war-retreat-default').bounding_box();assert box and box['x']>=0 and box['x']+box['width']<=width,(mode,box)
     page.screenshot(path=str(folder/f'{mode}-{choice}.png'))
     facts['captures'].append({'mode':mode,'choice':choice,'options':options,'value':value,'box':box,'errors':list(errors)})
    ctx.close()
   browser.close()
  if args.mode=='after':
   log=[]
   for flags in [['--hold-to-last-man'],['--by-loyalty'],[],['--retreat-at-loss','0.5']]:
    result=cli(flags);log.append('$ keryx retreat-default '+' '.join(flags)+'\n'+result.stdout)
    assert (result.returncode!=0)==('--retreat-at-loss' in flags),(flags,result.stdout)
   (folder/'keryx.txt').write_text('\n'.join(log))
   req=urllib.request.Request(BASE+'/api/v1/'+path,method='PUT',data=b'{"retreat_at_loss":0.5}',headers={'Content-Type':'application/json','Authorization':'Bearer '+token})
   try:urllib.request.urlopen(req);raise AssertionError('retired percentage accepted')
   except urllib.error.HTTPError as e:
    assert e.code==400;evidence=json.load(e);facts['retired_request']={'status':e.code,'body':evidence}
 finally:
  if args.mode=='after':
   body={'hold_to_last_man':True} if initial['hold_to_last_man'] else {'by_loyalty':True}
   req=urllib.request.Request(BASE+'/api/v1/'+path,method='PUT',data=json.dumps(body).encode(),headers={'Content-Type':'application/json','Authorization':'Bearer '+token})
   with urllib.request.urlopen(req) as r:facts['restored']=json.load(r)
assert all(not x['errors'] for x in facts['captures']),facts
(folder/'facts.json').write_text(json.dumps(facts,indent=2)+'\n')
print(args.mode,health,'Firefox1280+390; normal login/web/CLI/API, no SQL writes')
