#!/usr/bin/env python3
"""Next citizen proof: fresh owned rig, real API and Firefox1280/390.
Usage: python3 tools/nasta_medborgare_bild_20261024.py OUTPUT
Explicit scene fixtures are logged. No shared acceptance or production state.
"""
from pathlib import Path
import os,json,secrets,socket,subprocess,tempfile,time,urllib.request,urllib.error,hashlib,sys
from playwright.sync_api import sync_playwright
ROOT=Path(__file__).resolve().parents[1]
OUT=Path(sys.argv[1]).resolve();OUT.mkdir(parents=True,exist_ok=True)
containers=[];proc=None
clean={k:os.environ[k] for k in ('HOME','PATH')};clean['CGO_ENABLED']='0'
def run(*args,**kw):return subprocess.check_output(args,text=True,**kw).strip()
def hashes():return {str(p.relative_to(ROOT)):hashlib.sha256(p.read_bytes()).hexdigest() for d in ('server','web') for p in sorted((ROOT/d).rglob('*')) if p.is_file() and p.suffix in ('.go','.js','.md','.html','.sql','.css','.json') and not p.name.endswith('_test.go')}
before=hashes();fixtures=[];shots=[];observations=[]
def launch(kind,image,args):
 name='megaron-citizen-'+secrets.token_hex(4)+'-'+kind;internal='5432' if kind=='pg' else '6379'
 run('docker','run','-d','--rm','--name',name,'-p','127.0.0.1::'+internal,*args,image);containers.append(name)
 return json.loads(run('docker','inspect',name))[0]['NetworkSettings']['Ports'][internal+'/tcp'][0]['HostPort']
try:
 with tempfile.TemporaryDirectory(prefix='bildplan-proof-') as td:
  tmp=Path(td);commit=run('git','-C',str(ROOT),'rev-parse','--short=8','HEAD')
  run('go','build','-ldflags','-X main.buildCommit='+commit,'-o',str(tmp/'temenos'),'./cmd/server',cwd=ROOT/'server',env=clean)
  run('go','build','-o',str(tmp/'keryx'),'./cmd/keryx',cwd=ROOT/'server',env=clean)
  pg=launch('pg','postgres:16-alpine',['-e','POSTGRES_PASSWORD=bildplan','-e','POSTGRES_DB=bildplan']);redis=launch('redis','redis:7-alpine',[])
  for _ in range(100):
   if subprocess.run(['docker','exec',containers[0],'pg_isready','-U','postgres','-q'],capture_output=True).returncode==0:break
   time.sleep(.25)
  with socket.socket() as s:s.bind(('127.0.0.1',0));port=s.getsockname()[1]
  base=f'http://127.0.0.1:{port}'
  env={**clean,'DATABASE_URL':f'postgres://postgres:bildplan@localhost:{pg}/bildplan?sslmode=disable','REDIS_URL':'localhost:'+redis,'JWT_SECRET':secrets.token_urlsafe(40),'PORT':str(port),'TICK_SECONDS':'1800','MAP_WIDTH':'56','MAP_HEIGHT':'40','WORLD_NAME':'Lato visual proof','POLEIA_WORLD_START_WANAXES':'1','STATIC_DIR':str(ROOT/'web/static'),'TEMPLATE_DIR':str(ROOT/'web/templates'),'CHRONICLE_DIR':str(tmp/'chronicles'),'REPORTS_DIR':str(tmp/'reports')}
  assert not (ROOT/'server/.env').exists()
  proc=subprocess.Popen([str(tmp/'temenos')],cwd=ROOT/'server',env=env,stdout=(OUT/'server.log').open('w'),stderr=subprocess.STDOUT)
  def api(path,method='GET',data=None,token=None):
   req=urllib.request.Request(base+path,method=method,data=None if data is None else json.dumps(data).encode(),headers={'Content-Type':'application/json',**({'Authorization':'Bearer '+token} if token else {})})
   return json.load(urllib.request.urlopen(req,timeout=30))
  for _ in range(120):
   try:health=api('/healthz');break
   except Exception:
    if proc.poll() is not None:raise RuntimeError('server exited')
    time.sleep(.5)
  assert health['commit']==commit and health['migration']==210,health
  world=api('/api/v1/worlds')[0]['id'];wp='/api/v1/worlds/'+world
  def sql(q,fixture=False):
   if fixture:fixtures.append(q);(OUT/'fixture.sql').write_text('\n\n'.join(fixtures)+'\n')
   return run('docker','exec',containers[0],'psql','-U','postgres','-d','bildplan','-At','-v','ON_ERROR_STOP=1','-c',q)
  def actor(name):
   token=api('/api/v1/auth/register','POST',{'username':name+'-'+secrets.token_hex(3),'password':secrets.token_urlsafe(30)})['access_token']
   api(wp+'/join','POST',{},token);pid=api('/api/v1/auth/me',token=token)['id']
   host=next(u for u in api(wp+'/units',token=token)['units'] if u['type']=='nomadic_host')
   sql(f"UPDATE players SET wanax_name='{name}' WHERE id='{pid}'",True)
   return dict(name=name,token=token,pid=pid,host=host['id'])
  me=actor('Leandros')
  site=json.loads(sql(f"SELECT row_to_json(t) FROM(SELECT q,r FROM map_tiles WHERE world_id='{world}' AND terrain='plains' ORDER BY q,r LIMIT 1)t"));q,r=site['q'],site['r']
  sql(f"UPDATE units SET q={q},r={r},status='positioned' WHERE id='{me['host']}';",True)
  me['sid']=api(wp+'/founding/settle','POST',{'name':'Lato'},me['token'])['settlement_id']
  me['province']=next(p['id'] for p in api(wp+'/provinces',token=me['token']) if p.get('settlement_id')==me['sid'])
  sql(f"UPDATE worlds SET current_tick=100,last_tick_at=now() WHERE id='{world}'; UPDATE units SET q={q},r={r},status='garrison',settlement_id='{me['sid']}',support_settlement_id='{me['sid']}' WHERE world_id='{world}' AND owner_id='{me['pid']}' AND type='spearman';",True)
  config=tmp/'keryx.json';config.write_text(json.dumps({'server':base,'token':me['token'],'world_id':world,'province_id':me['province']}))
  cases=[('G','growing',1964,1,100,0,'in about 4 game days (about 2 hours)'),('H','holding',1964,1,0,0,'not growing'),('C','capped',2000,0,100,0,'the walls are full'),('S','shrinking',1964,1,0,1,'shrinking')]
  def fixture(pop,wall,rate,unmet):
   sql(f"UPDATE worlds SET last_tick_at=now() WHERE id='{world}'; UPDATE settlements SET population={pop},wall_level={wall},food_unmet_amount={unmet},food_consumed='{{\"grain\":10}}',food_consumed_tick=100 WHERE id='{me['sid']}'; UPDATE settlement_goods SET amount=CASE WHEN {unmet}>0 AND good_key IN('grain','fish','oil','wine','livestock') THEN 0 WHEN good_key='grain' THEN 500 WHEN good_key='livestock' THEN 10 ELSE 100 END,rate=CASE WHEN good_key='grain' THEN {rate} ELSE 0 END,cap=1000000,calc_tick=100 WHERE settlement_id='{me['sid']}';",True)
  print('RIG READY '+base+' health='+json.dumps(health),flush=True)
  with sync_playwright() as pw:
   browser=pw.firefox.launch()
   for width,height in [(1280,900),(390,844)]:
    ctx=browser.new_context(viewport={'width':width,'height':height})
    ctx.add_cookies([{'name':'poleia_token','value':me['token'],'url':base}]);ctx.add_init_script('localStorage.setItem("poleia_token",'+json.dumps(me['token'])+')')
    page=ctx.new_page();errors=[];page.on('pageerror',lambda e:errors.append(str(e)))
    for code,state,pop,wall,rate,unmet,expected in cases:
     fixture(pop,wall,rate,unmet)
     snapshot=api(wp+'/provinces/'+me['province'],token=me['token'])
     pd=snapshot['settlement'];assert pd['food']['growth_state']==state,pd['food']
     assert pd['next_citizen_progress']==pop%100,pd
     if state=='growing':assert pd['next_citizen_in_ticks']==4 and pd['next_citizen_growth_per_tick']==10,pd
     else:assert pd['next_citizen_in_ticks'] is None and pd['next_citizen_growth_per_tick']==0,pd
     (OUT/f'{code}-{width}-api.json').write_text(json.dumps(snapshot,indent=2))
     for command in ['city','status']:
      cli=run(str(tmp/'keryx'),command,env={**clean,'POLEIA_CONFIG':str(config)})
      assert 'Next citizen' in cli and expected in cli,cli
      (OUT/f'{code}-{width}-keryx-{command}.txt').write_text(cli+'\n')
     page.goto(base+'/world/'+world+'/map',wait_until='networkidle')
     if page.locator('#dc-dismiss-all').is_visible():page.locator('#dc-dismiss-all').click()
     for brief in page.locator('.drawer.open .lb-dismiss').all():
      if brief.is_visible():brief.click()
     page.evaluate("()=>window.openDrawer('city')")
     page.locator('.citizen-progress').wait_for()
     for brief in page.locator('#drawer-city .lb-dismiss').all():
      if brief.is_visible():brief.click()
     text=page.locator('.next-citizen').inner_text()
     assert expected in text,text
     assert page.locator('.citizen-step').count()==10
     assert page.locator('.citizen-step.filled').count()==(pop%100)//10
     page.locator('.next-citizen').scroll_into_view_if_needed();page.wait_for_timeout(200)
     image=OUT/f'{code}-{width}.png';page.screenshot(path=str(image));shots.append({'code':code+str(width),'file':image.name,'width':width,'state':state,'text':text})
     assert not errors,errors
     observations.append({'width':width,'state':state,'page_errors':list(errors),'progress':pd['next_citizen_progress'],'growth':pd['next_citizen_growth_per_tick'],'eta':pd['next_citizen_in_ticks']})
    ctx.close()
   browser.close()
  assert before==hashes(),'Production sources changed during capture'
  (OUT/'proof.json').write_text(json.dumps({'health':health,'fixture':'Actual provinceAPI and City drawer plus compiled Keryx city/status. Fresh owned rig with genuinely founded city, explicit population/wall/stock/rate/meal fixtures per case; no mocked responses or DOM replacement.','source_sha256':before,'source_unchanged':before==hashes(),'observations':observations,'shots':shots},indent=2))
  print('PASS next citizen:4states×2widths Firefox +actualAPI/Keryx',flush=True)
finally:
 if proc is not None:proc.terminate();proc.wait(timeout=15)
 for name in containers:subprocess.run(['docker','rm','-fv',name],capture_output=True)
