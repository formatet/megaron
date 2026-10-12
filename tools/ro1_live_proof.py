#!/usr/bin/env python3
"""Fresh isolated runtime: natural join, Host forecast in API/Keryx/web, real march.
No SQL fixtures, writes, HTTP overrides, or shared acceptance runtime.
"""
import json, os, pathlib, secrets, socket, subprocess, tempfile, time, urllib.request
ROOT=pathlib.Path(__file__).resolve().parents[1]
OUT=ROOT/'docs/reviews/rorelsetabellen/live'
OUT.mkdir(parents=True,exist_ok=True)
commit=subprocess.check_output(['git','-C',str(ROOT),'rev-parse','--short=7','HEAD'],text=True).strip()
containers=[];proc=None;browser=None;playwright=None
prefix='megaron-ro1-'+secrets.token_hex(4)
def run(*args,**kw):return subprocess.check_output(args,text=True,**kw).strip()
def launch(service,image,args):
 name=prefix+'-'+service;port='5432' if service=='pg' else '6379'
 run('docker','run','-d','--rm','--name',name,'-p','127.0.0.1::'+port,*args,image);containers.append(name)
 bindings=json.loads(run('docker','inspect',name))[0]['NetworkSettings']['Ports']
 return int(bindings[port+'/tcp'][0]['HostPort'])
try:
 with tempfile.TemporaryDirectory(prefix='megaron-ro1-live-') as tmp:
  tmp=pathlib.Path(tmp)
  clean={k:os.environ[k] for k in ('HOME','PATH')};clean['CGO_ENABLED']='0'
  run('go','build','-ldflags','-X main.buildCommit='+commit,'-o',str(tmp/'temenos'),'./cmd/server',cwd=ROOT/'server',env=clean)
  run('go','build','-o',str(tmp/'keryx'),'./cmd/keryx',cwd=ROOT/'server',env=clean)
  pg=launch('pg','postgres:16-alpine',['-e','POSTGRES_PASSWORD=ro1-proof-pw','-e','POSTGRES_DB=ro1_proof'])
  redis=launch('redis','redis:7-alpine',[])
  for _ in range(80):
   if subprocess.run(['docker','exec',containers[0],'pg_isready','-U','postgres','-q'],capture_output=True).returncode==0:break
   time.sleep(.25)
  with socket.socket() as sock:sock.bind(('127.0.0.1',0));port=sock.getsockname()[1]
  base=f'http://127.0.0.1:{port}'
  env={**clean,'DATABASE_URL':f'postgres://postgres:ro1-proof-pw@localhost:{pg}/ro1_proof?sslmode=disable','REDIS_URL':f'localhost:{redis}',
   'JWT_SECRET':secrets.token_urlsafe(40),'PORT':str(port),'TICK_SECONDS':'60','MAP_WIDTH':'64','MAP_HEIGHT':'48',
   'WORLD_NAME':'RO1 proof','POLEIA_WORLD_START_WANAXES':'1','STATIC_DIR':str(ROOT/'web/static'),'TEMPLATE_DIR':str(ROOT/'web/templates'),
   'CHRONICLE_DIR':str(tmp/'chronicles'),'REPORTS_DIR':str(tmp/'reports')}
  assert not (ROOT/'server/.env').exists()
  proc=subprocess.Popen([str(tmp/'temenos')],cwd=ROOT/'server',env=env,stdout=(OUT/'server.log').open('w'),stderr=subprocess.STDOUT)
  def api(path,method='GET',data=None,token=None):
   req=urllib.request.Request(base+path,method=method,data=None if data is None else json.dumps(data).encode(),headers={'Content-Type':'application/json',**({'Authorization':'Bearer '+token} if token else {})})
   with urllib.request.urlopen(req,timeout=30) as response:return json.load(response)
  for _ in range(120):
   try:health=api('/healthz');break
   except Exception:
    if proc.poll() is not None:raise RuntimeError('server exited')
    time.sleep(.5)
  else:raise RuntimeError('server timeout')
  assert health['commit']==commit and health['migration']==174,health
  ws=api('/api/v1/worlds');world=(ws if isinstance(ws,list) else ws['worlds'])[0]['id'];wp='/api/v1/worlds/'+world
  target=None
  for attempt in range(20):
   username='RO1-'+secrets.token_hex(4)
   token=api('/api/v1/auth/register','POST',{'username':username,'password':secrets.token_urlsafe(30)})['access_token']
   api(wp+'/join','POST',{},token)
   host=next(u for u in api(wp+'/units',token=token)['units'] if u['type']=='nomadic_host')
   seen=api(wp+'/map',token=token);seen=seen if isinstance(seen,list) else seen['tiles']
   tiles={(t['q'],t['r']):t for t in seen}
   q,r=host['q'],host['r']
   for dq,dr in [(1,0),(1,-1),(0,-1),(-1,0),(-1,1),(0,1)]:
    if all(tiles.get((q+n*dq,r+n*dr),{}).get('terrain') in ('plains','river_valley','river_delta') for n in (1,2)):
     target=(q+2*dq,r+2*dr);break
   if target:break
  assert target,'no naturally known two-step plains corridor'
  unitpath=wp+'/units/'+host['id'];query=f'?target_q={target[0]}&target_r={target[1]}'
  forecast=api(unitpath+'/march-preview'+query,token=token)
  assert forecast['available'] and forecast['duration_ticks']==4,forecast
  cfg=tmp/'config.json';cfg.write_text(json.dumps({'server':base,'world_id':world,'token':token}));cfg.chmod(0o600)
  clienv={**clean,'POLEIA_CONFIG':str(cfg)}
  cliargs=[str(tmp/'keryx'),'march','--unit',host['id'],'--target',f'{target[0]},{target[1]}','--preview']
  cli=run(*cliargs,env=clienv);raw=json.loads(run(*cliargs,'--json',env=clienv))
  assert raw['duration_ticks']==4 and '4 days' in cli,(raw,cli)
  (OUT/'keryx-preview.txt').write_text(cli+'\n');(OUT/'keryx-preview.json').write_text(json.dumps(raw,indent=2)+'\n')
  from playwright.sync_api import sync_playwright
  playwright=sync_playwright().start();browser=playwright.firefox.launch();page=browser.new_page()
  page.goto(base+'/',wait_until='domcontentloaded')
  page.evaluate('t=>localStorage.setItem("poleia_token",t)',token)
  page.context.add_cookies([{'name':'poleia_token','value':token,'url':base}])
  errors=[];page.on('pageerror',lambda e:errors.append(str(e)))
  page.goto(base+'/world/'+world,wait_until='domcontentloaded')
  # Use the production web forecast module with its real Bearer client. No
  # supplied payload, monkeypatch, or replacement of the request/formatter.
  web=page.evaluate('''async ({world,id,q,r})=>{
   const preview=await import('/static/js/megaron/ui/march_preview.js');
   const api=await import('/static/js/megaron/api.js');
   const data=await preview.loadMarchPreview(world,{id},{target_q:q,target_r:r},api.fetchAuth);
   return {data,html:preview.marchPreviewHTML(data)};
  }''',{'world':world,'id':host['id'],'q':target[0],'r':target[1]})
  assert web['data']['duration_ticks']==4 and '4 days' in web['html'],web
  receipt=api(unitpath+'/march','POST',{'target_q':target[0],'target_r':target[1]},token)
  assert receipt['duration_ticks']==4,receipt
  units=api(wp+'/units',token=token)['units'];marching=next(u for u in units if u['id']==host['id'])
  assert marching['status']=='marching',marching
  (OUT/'verification.json').write_text(json.dumps({'health':health,'world':world,'username':username,'registration_attempts':attempt+1,'host':host['id'],'origin':[q,r],'target':target,'api_preview':forecast,'cli_preview':raw,'web_preview':web,'march_receipt':receipt,'after_dispatch':marching,'browser_errors':errors,'sql_fixtures':False,'payload_overrides':False,'scope':'Natural joined Host, two known plains steps, expected 4 ticks vs pre-RO1 3. Production web module and formatter called in Firefox; this is a functional proof, not a UI discoverability/BILD verdict.'},indent=2)+'\n')
  print('API/Keryx/Firefox forecast 4 days, real Host march 4 ticks; normal join, no SQL writes')
finally:
 if browser:browser.close()
 if playwright:playwright.stop()
 if proc:
  proc.terminate()
  try:proc.wait(timeout=10)
  except subprocess.TimeoutExpired:proc.kill();proc.wait()
 for name in containers:subprocess.run(['docker','rm','-fv',name],capture_output=True)
