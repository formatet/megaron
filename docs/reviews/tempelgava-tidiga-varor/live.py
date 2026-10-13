#!/usr/bin/env python3
"""Natural register/join/found, produce resources, build temple and observe worker.
All gameplay actions via API/Keryx; SQL reads only. Firefox1280/390 screenshots.
"""
import hashlib,json,os,pathlib,secrets,socket,subprocess,tempfile,time,urllib.request,urllib.error
HERE=pathlib.Path(__file__).resolve().parent;ROOT=HERE.parents[2]
OUT=HERE/os.environ.get('TEMPLE_NATIVE_OUT','native');OUT.mkdir(parents=True,exist_ok=True)
def run(*a,**kw):return subprocess.check_output(a,text=True,**kw).strip()
commit=run('git','-C',str(ROOT),'rev-parse','--short=8','HEAD')
diff=hashlib.sha256(subprocess.check_output(['git','-C',str(ROOT),'diff','HEAD','--','server','web'])).hexdigest()
assert run('git','-C',str(ROOT),'diff','HEAD','--','server','web')=='', 'commit production before native proof'
containers=[];proc=browser=pw=None
prefix='temple-native-'+secrets.token_hex(4)
def launch(service,image,args):
 name=prefix+'-'+service;port='5432' if service=='pg' else '6379'
 run('docker','run','-d','--rm','--name',name,'-p','127.0.0.1::'+port,*args,image);containers.append(name)
 return int(json.loads(run('docker','inspect',name))[0]['NetworkSettings']['Ports'][port+'/tcp'][0]['HostPort'])
try:
 with tempfile.TemporaryDirectory(prefix='temple-native-') as tmp:
  tmp=pathlib.Path(tmp);clean={k:os.environ[k] for k in ('HOME','PATH')};clean['CGO_ENABLED']='0'
  run('go','build','-ldflags','-X main.buildCommit='+commit,'-o',str(tmp/'temenos'),'./cmd/server',cwd=ROOT/'server',env=clean)
  run('go','build','-o',str(tmp/'keryx'),'./cmd/keryx',cwd=ROOT/'server',env=clean)
  pg=launch('pg','postgres:16-alpine',['-e','POSTGRES_PASSWORD=native-proof','-e','POSTGRES_DB=native_proof'])
  redis=launch('redis','redis:7-alpine',[])
  for _ in range(120):
   if subprocess.run(['docker','exec',containers[0],'pg_isready','-U','postgres','-q'],capture_output=True).returncode==0:break
   time.sleep(.25)
  time.sleep(1)
  with socket.socket() as sock:sock.bind(('127.0.0.1',0));port=sock.getsockname()[1]
  base=f'http://127.0.0.1:{port}'
  env={**clean,'DATABASE_URL':f'postgres://postgres:native-proof@localhost:{pg}/native_proof?sslmode=disable','REDIS_URL':f'localhost:{redis}',
   'JWT_SECRET':secrets.token_urlsafe(40),'PORT':str(port),'TICK_SECONDS':'1','MAP_WIDTH':'64','MAP_HEIGHT':'64',
   'WORLD_NAME':'Temple proof','POLEIA_WORLD_START_WANAXES':'1','STATIC_DIR':str(ROOT/'web/static'),'TEMPLATE_DIR':str(ROOT/'web/templates'),
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
  token=api('/api/v1/auth/register','POST',{'username':'Temple-'+secrets.token_hex(4),'password':secrets.token_urlsafe(30)})['access_token']
  api(wp+'/join','POST',{},token)
  def sql(query):
   data=subprocess.check_output(['docker','exec','-i',containers[0],'psql','-U','postgres','-d','native_proof','-qAt','-v','ON_ERROR_STOP=1'],input='BEGIN READ ONLY;'+query+';COMMIT;',text=True)
   return json.loads(data.strip()) if data.strip() else None
  selected=None; trials=[]
  for attempt in range(8):
   if attempt:
    token=api('/api/v1/auth/register','POST',{'username':'Temple-'+secrets.token_hex(4),'password':secrets.token_urlsafe(30)})['access_token']
    api(wp+'/join','POST',{},token)
   founded=api(wp+'/founding/settle','POST',{'name':'Phaistos'},token)
   city=next(m for m in api(wp+'/provinces',token=token) if m.get('own'))
   province=city['id'];settlement=city['settlement_id'];pp=wp+'/provinces/'+province
   options=api(pp+'/placement-options',token=token)
   timber=[(h,g) for h in options['hexes'] for g in h['goods'] if g['good_key']=='timber' and g.get('cap',0)>g['placed'] and not h.get('held_by')]
   stone=[(h,g) for h in options['hexes'] for g in h['goods'] if g['good_key']=='stone' and g.get('cap',0)>g['placed'] and not h.get('held_by')]
   trials.append({'attempt':attempt,'founded':founded,'timber_options':len(timber),'stone_options':len(stone)})
   if timber and stone:
    selected=(timber[0],max(stone,key=lambda x:x[1]['marginal_yield']));break
  assert selected, trials
  placements=api(pp+'/placements',token=token)
  for good,(hx,g) in zip(('timber','stone'),selected):
   options=api(pp+'/placement-options',token=token)
   if options['pool_size']==0:
    current=api(pp+'/placements',token=token)
    placed=current['placements']
    donor=next(p for p in placed if p['good_key']=='grain')
    api(pp+'/placements/'+str(donor['gubbe_ordinal']),'DELETE',token=token)
   api(pp+'/placements','POST',{'target_kind':'hex','hex_q':hx['hex_q'],'hex_r':hx['hex_r'],'good_key':good},token)
  catalogue=api('/api/v1/buildings')
  temple=next(b for b in catalogue if b['type']=='temple')
  deadline=time.monotonic()+120
  while time.monotonic()<deadline:
   goods={g['key']:g['amount'] for g in api(pp+'/goods',token=token)}
   if all(goods.get(g,0)>=amount for g,amount in temple['costs'].items()):break
   time.sleep(.5)
  else:raise AssertionError(('production did not reach temple costs',goods,temple))
  built=api(pp+'/build','POST',{'building_type':'temple'},token)
  deadline=time.monotonic()+60
  while time.monotonic()<deadline:
   status=api(pp,token=token)['settlement']
   if status['temple_offers']:break
   time.sleep(.5)
  else:raise AssertionError('natural temple did not complete')
  api(pp+'/labor','PUT',{'percent':{'cult':15}},token)
  status=api(pp,token=token)['settlement'];offer=status['temple_offers'][0]
  assert offer['fed'] and offer['required_goods']=={'grain':.25} and offer['offering'],offer
  assert offer['stock'].get('oil',0)==0 and offer['stock'].get('wine',0)==0,offer
  eid=sql("SELECT COALESCE(MAX(id),0) FROM events WHERE world_id='"+world+"' AND event_type='KharisOffering'")
  deadline=time.monotonic()+20
  while time.monotonic()<deadline:
   event=sql("SELECT row_to_json(e) FROM (SELECT id,payload FROM events WHERE world_id='"+world+"' AND stream_id='"+settlement+"' AND event_type='KharisOffering' AND id>"+str(eid)+" ORDER BY id DESC LIMIT 1) e")
   if event and event['payload']['temples_fed']>0 and event['payload']['kharis_gain']>0:break
   time.sleep(.25)
  else:raise AssertionError('real worker did not feed early temple')
  cfg=tmp/'config.json';cfg.write_text(json.dumps({'server':base,'world_id':world,'province_id':province,'token':token}));cfg.chmod(0o600)
  cli=run(str(tmp/'keryx'),'status',env={**clean,'POLEIA_CONFIG':str(cfg)})
  assert 'reference 0.25 grain per day' in cli and 'next gift:' in cli,cli
  (OUT/'keryx-status.txt').write_text(cli+'\n')
  from playwright.sync_api import sync_playwright
  pw=sync_playwright().start();browser=pw.firefox.launch();captures=[]
  for mode,width,height in [('desktop',1280,900),('mobile',390,844)]:
   context=browser.new_context(viewport={'width':width,'height':height})
   context.add_cookies([{'name':'poleia_token','value':token,'url':base}]);context.add_init_script('localStorage.setItem("poleia_token",'+json.dumps(token)+')')
   page=context.new_page();errors=[];page.on('pageerror',lambda e:errors.append(str(e)))
   page.goto(base+'/world/'+world+'/map',wait_until='networkidle')
   if page.locator('#dc-dismiss-all').is_visible():page.locator('#dc-dismiss-all').click()
   for brief in page.locator('.lb-dismiss').all():
    if brief.is_visible():brief.click()
   page.evaluate("openDrawer('kult')")
   page.wait_for_function("document.getElementById('kult-body')?.textContent.includes('Next gift:')")
   page.evaluate("async()=>await Promise.all([...document.querySelectorAll('.drawer.open')].flatMap(d=>d.getAnimations()).map(a=>a.finished.catch(()=>{})))")
   text=page.locator('#kult-body').inner_text()
   assert 'Phaistos:' in text and 'reference 0.25 grain per day' in text and 'Next gift:' in text and 'Phaistosneeds' not in text,text
   page.locator('#kult-body .dsec').filter(has_text='Temple offerings').scroll_into_view_if_needed()
   rect=page.locator('#kult-body .dsec').filter(has_text='Temple offerings').evaluate('(el)=>({width:el.clientWidth,scroll:el.scrollWidth})')
   assert rect['scroll']<=rect['width']+1,rect
   page.screenshot(path=str(OUT/(mode+'.png')))
   captures.append({'mode':mode,'text':text,'errors':errors,'offering_rect':rect})
   context.close()
  assert all(not c['errors'] for c in captures),captures
  (OUT/'verification.json').write_text(json.dumps({'health':health,'trials':trials,'natural_resources':goods,'catalogue_temple':temple,'build_response':built,'status_offer':offer,'worker_event':event,'captures':captures,'sql_writes':False,'payload_overrides':False,'production_diff_sha256':diff},indent=2)+'\n')
  print('PASS natural founding/production/build + actual worker fed '+str(event['payload']['kharis_gain'])+' kharis; Keryx + native Firefox1280/390; SQL READ ONLY')
finally:
 if proc:
  proc.terminate()
  try:proc.wait(timeout=10)
  except subprocess.TimeoutExpired:proc.kill();proc.wait()
 for name in containers:subprocess.run(['docker','rm','-fv',name],capture_output=True)
