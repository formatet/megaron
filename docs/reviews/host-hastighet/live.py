#!/usr/bin/env python3
"""Natural register/join; actual Host march preview API, CLI and Firefox.
No SQL fixture/writes, request interception, supplied payloads or CSS overrides.
"""
import hashlib,json,os,pathlib,secrets,socket,subprocess,tempfile,time,urllib.request
HERE=pathlib.Path(__file__).resolve().parent;ROOT=HERE.parents[2]
OUT=HERE/os.environ.get('HOST_PROOF_OUT','native');OUT.mkdir(parents=True,exist_ok=True)
def run(*a,**kw):return subprocess.check_output(a,text=True,**kw).strip()
commit=run('git','-C',str(ROOT),'rev-parse','--short=8','HEAD')
diff=hashlib.sha256(subprocess.check_output(['git','-C',str(ROOT),'diff','HEAD','--','server','web'])).hexdigest()
assert run('git','-C',str(ROOT),'diff','HEAD','--','server','web')=='', 'commit production before native proof'
containers=[];proc=browser=pw=None
prefix='march-live-'+secrets.token_hex(4)
def launch(service,image,args):
 name=prefix+'-'+service;port='5432' if service=='pg' else '6379'
 run('docker','run','-d','--rm','--name',name,'-p','127.0.0.1::'+port,*args,image);containers.append(name)
 return int(json.loads(run('docker','inspect',name))[0]['NetworkSettings']['Ports'][port+'/tcp'][0]['HostPort'])
try:
 with tempfile.TemporaryDirectory(prefix='march-native-') as tmp:
  tmp=pathlib.Path(tmp);clean={k:os.environ[k] for k in ('HOME','PATH')};clean['CGO_ENABLED']='0'
  run('go','build','-ldflags','-X main.buildCommit='+commit,'-o',str(tmp/'temenos'),'./cmd/server',cwd=ROOT/'server',env=clean)
  run('go','build','-o',str(tmp/'keryx'),'./cmd/keryx',cwd=ROOT/'server',env=clean)
  pg=launch('pg','postgres:16-alpine',['-e','POSTGRES_PASSWORD=native-proof','-e','POSTGRES_DB=native_proof'])
  redis=launch('redis','redis:7-alpine',[])
  for _ in range(120):
   if subprocess.run(['docker','exec',containers[0],'pg_isready','-h','127.0.0.1','-U','postgres','-q'],capture_output=True).returncode==0:break
   time.sleep(.25)
  time.sleep(1)
  with socket.socket() as sock:sock.bind(('127.0.0.1',0));port=sock.getsockname()[1]
  base=f'http://127.0.0.1:{port}'
  env={**clean,'DATABASE_URL':f'postgres://postgres:native-proof@localhost:{pg}/native_proof?sslmode=disable','REDIS_URL':f'localhost:{redis}',
   'JWT_SECRET':secrets.token_urlsafe(40),'PORT':str(port),'TICK_SECONDS':'3600','MAP_WIDTH':'64','MAP_HEIGHT':'64',
   'WORLD_NAME':'Spear proof','POLEIA_WORLD_START_WANAXES':'1','STATIC_DIR':str(ROOT/'web/static'),'TEMPLATE_DIR':str(ROOT/'web/templates'),
   'CHRONICLE_DIR':str(tmp/'chronicles'),'REPORTS_DIR':str(tmp/'reports')}
  assert not (ROOT/'server/.env').exists()
  proc=subprocess.Popen([str(tmp/'temenos')],cwd=ROOT/'server',env=env,stdout=(OUT/'server.log').open('w'),stderr=subprocess.STDOUT)
  def api(path,method='GET',data=None,token=None):
   req=urllib.request.Request(base+path,method=method,data=None if data is None else json.dumps(data).encode(),headers={'Content-Type':'application/json',**({'Authorization':'Bearer '+token} if token else {})})
   with urllib.request.urlopen(req,timeout=30) as response:return json.load(response)
  for _ in range(120):
   try:health=api('/healthz');break
   except Exception:
    if proc.poll() is not None:raise RuntimeError('native server exited')
    time.sleep(.5)
  else:raise RuntimeError('native startup timeout')
  assert health['commit']==commit and health['migration']==179,health
  worlds=api('/api/v1/worlds');world=(worlds if isinstance(worlds,list) else worlds['worlds'])[0]['id'];wp='/api/v1/worlds/'+world
  token=api('/api/v1/auth/register','POST',{'username':'Spear-'+secrets.token_hex(4),'password':secrets.token_urlsafe(30)})['access_token']
  api(wp+'/join','POST',{},token)
  founder=api(wp+'/founding/status',token=token)
  units=api(wp+'/units',token=token)['units'];host=next(u for u in units if u['type']=='nomadic_host')
  tiles=api(wp+'/map',token=token);tiles=tiles if isinstance(tiles,list) else tiles['tiles']
  costs={'plains':1,'river_valley':1,'river_delta':1,'forest_olive_grove':1.5,'scrub_maquis':1.5,'forest_cedar':2,'hills':2,'semi_desert':3}
  def distance(t):
   dq=t['q']-founder['q'];dr=t['r']-founder['r'];return max(abs(dq),abs(dr),abs(dq+dr))
  candidates=[t for t in tiles if distance(t)==1 and t['terrain'] in costs]
  assert candidates,('no passable neighbouring land',founder)
  tile=max(candidates,key=lambda t:costs[t['terrain']]);q,r=tile['q'],tile['r']
  target={'q':q,'r':r,'known':True,'name':tile['terrain'],'isSea':False,'isSettlement':False}
  expected=api(wp+'/units/'+host['id']+f'/march-preview?target_q={q}&target_r={r}',token=token)
  want=max(1,int(costs[tile['terrain']]/.9+.5))
  assert expected['available'] and expected['duration_ticks']==want,(expected,tile,want)
  cfg=tmp/'config.json';cfg.write_text(json.dumps({'server':base,'world_id':world,'token':token}));cfg.chmod(0o600)
  cli_env={**clean,'POLEIA_CONFIG':str(cfg)}
  args=['unit','march','--unit',host['id'],'--q',str(q),'--r',str(r)]
  cli=run(str(tmp/'keryx'),*args,'--preview',env=cli_env)
  journey=f"journey: {want} "+('day' if want==1 else 'days')
  assert journey in cli and f"day {expected['arrival_tick']}" in cli,cli
  (OUT/'keryx-preview.txt').write_text(cli+'\n')
  before_preview=api(wp+'/units',token=token)
  from playwright.sync_api import sync_playwright
  pw=sync_playwright().start();browser=pw.firefox.launch();captures=[]
  for mode,width,height in [('desktop',1280,900),('mobile',390,844)]:
   ctx=browser.new_context(viewport={'width':width,'height':height})
   ctx.add_cookies([{'name':'poleia_token','value':token,'url':base}]);ctx.add_init_script('localStorage.setItem("poleia_token",'+json.dumps(token)+')')
   page=ctx.new_page();errors=[];responses=[];page.on('pageerror',lambda e:errors.append(str(e)))
   def record(response):
    if '/units/'+host['id']+'/march-preview?' in response.url and response.status==200:responses.append(response.json())
   page.on('response',record)
   page.goto(base+'/world/'+world+'/map',wait_until='networkidle')
   if page.locator('#dc-dismiss-all').is_visible():page.locator('#dc-dismiss-all').click()
   for brief in page.locator('.lb-dismiss').all():
    if brief.is_visible():brief.click()
   page.evaluate('async dest=>await openMarchCtx(dest,8,80)',target)
   page.locator('#mctx-units .mctx-input').first.wait_for()
   for row in page.locator('#mctx-units .mctx-row').all():
    row.locator('input').fill('1' if 'Nomadic Host' in row.inner_text() else '0')
   page.wait_for_function('expected=>document.querySelector("#mctx-eta")?.textContent.includes(expected)',arg=f"{want} "+('day' if want==1 else 'days')+' travelling')
   assert responses and responses[-1]['arrival_tick']==expected['arrival_tick'] and responses[-1]['duration_ticks']==want,responses
   eta=page.locator('#mctx-eta');text=eta.inner_text()
   assert 'Nomadic Host' in text and f"{want} "+('day' if want==1 else 'days') in text,text
   rect=eta.bounding_box();assert rect and rect['x']>=0 and rect['x']+rect['width']<=width+1,rect
   assert eta.evaluate('(el)=>el.scrollWidth<=el.clientWidth+1')
   page.screenshot(path=str(OUT/(mode+'.png')))
   (OUT/(mode+'-preview.txt')).write_text(page.locator('#march-ctx').inner_text()+'\n')
   assert not errors,errors
   captures.append({'mode':mode,'viewport':[width,height],'text':text,'preview_response':responses[-1],'rect':rect,'page_errors':errors});ctx.close()
  assert api(wp+'/units',token=token)==before_preview,'preview changed units'
  receipt=json.loads(run(str(tmp/'keryx'),*args,'--json',env=cli_env))
  assert receipt['duration_ticks']==want and receipt['arrival_tick']==expected['arrival_tick'],receipt
  db=run('docker','exec',containers[0],'psql','-U','postgres','-d','native_proof','-X','-qAt','-c',"SELECT json_build_object('depart_tick',depart_tick,'arrive_tick',arrive_tick,'march_route',march_route) FROM units WHERE id='"+host['id']+"'")
  stored=json.loads(db);assert stored['arrive_tick']==expected['arrival_tick'] and stored['arrive_tick']-stored['depart_tick']==want,stored
  # Independent calculation from saved terrain costs; no client speed formula.
  import math
  assert max(1,math.floor(sum(stored['march_route']['costs'])/1000/.9+.5))==want,stored
  listed=api(wp+'/units',token=token);marching=next(u for u in listed['units'] if u['id']==host['id'])
  assert marching['arrival_tick']==expected['arrival_tick'] and marching['duration_ticks']==want,marching
  after=run('docker','exec',containers[0],'psql','-U','postgres','-d','native_proof','-X','-qAt','-c',"SELECT json_build_object('depart_tick',depart_tick,'arrive_tick',arrive_tick,'march_route',march_route) FROM units WHERE id='"+host['id']+"'")
  assert after==db,'reading stored march changed timing or route'
  (OUT/'verification.json').write_text(json.dumps({'health':health,'production_diff_sha256':diff,'natural_register_join':True,'sql_fixtures_or_writes':False,'request_overrides':False,'target':tile,'preview':expected,'receipt':receipt,'stored':stored,'listed_host':marching,'captures':captures,'read_only_preview_units':True,'stored_march_unchanged_on_read':True},indent=2)+'\n')
  print('PASS natural register/join → Host preview API=CLI=Firefox1280/390=dispatch=DB; normal rounding, stored march unchanged')

finally:
 if browser:browser.close()
 if pw:pw.stop()
 if proc:
  proc.terminate()
  try:proc.wait(timeout=10)
  except subprocess.TimeoutExpired:proc.kill();proc.wait()
 for name in containers:subprocess.run(['docker','rm','-fv',name],capture_output=True)
