#!/usr/bin/env python3
"""Natural register/join and persistent opening dispatch.
All gameplay actions via API/Keryx; SQL reads only. Firefox1280/390 screenshots.
"""
import hashlib,json,os,pathlib,secrets,socket,subprocess,tempfile,time,urllib.request,urllib.error
HERE=pathlib.Path(__file__).resolve().parent;ROOT=HERE.parents[2]
OUT=HERE/os.environ.get('OPENING_NATIVE_OUT','native');OUT.mkdir(parents=True,exist_ok=True)
def run(*a,**kw):return subprocess.check_output(a,text=True,**kw).strip()
commit=run('git','-C',str(ROOT),'rev-parse','--short=8','HEAD')
diff=hashlib.sha256(subprocess.check_output(['git','-C',str(ROOT),'diff','HEAD','--','server','web'])).hexdigest()
assert run('git','-C',str(ROOT),'diff','HEAD','--','server','web')=='', 'commit production before native proof'
containers=[];proc=browser=pw=None
prefix='opening-native-'+secrets.token_hex(4)
rise_enabled=os.environ.get('OPENING_NATURAL_RISE')=='1'
tick_seconds=1 if rise_enabled else 1800
start_wanaxes=2 if rise_enabled else 4
def launch(service,image,args):
 name=prefix+'-'+service;port='5432' if service=='pg' else '6379'
 run('docker','run','-d','--rm','--name',name,'-p','127.0.0.1::'+port,*args,image);containers.append(name)
 return int(json.loads(run('docker','inspect',name))[0]['NetworkSettings']['Ports'][port+'/tcp'][0]['HostPort'])
try:
 with tempfile.TemporaryDirectory(prefix='opening-native-') as tmp:
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
   'JWT_SECRET':secrets.token_urlsafe(40),'PORT':str(port),'TICK_SECONDS':str(tick_seconds),'MAP_WIDTH':'64','MAP_HEIGHT':'64',
   'WORLD_NAME':'Opening dispatch proof','POLEIA_WORLD_START_WANAXES':str(start_wanaxes),'STATIC_DIR':str(ROOT/'web/static'),'TEMPLATE_DIR':str(ROOT/'web/templates'),
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
  notes=api(wp+'/notifications?kind=OpeningDispatch',token=token)['notifications']
  assert len(notes)==1,notes
  note=notes[0];body=note['body']
  assert body['title']=='Your people await you' and len(body['paragraphs'])==8 and not body['returning'],body
  assert ('1 second' if rise_enabled else '30 minutes') in body['paragraphs'][0] and f'{start_wanaxes} Wanaxes' in body['paragraphs'][0],body
  repeat=api(wp+'/join','POST',{},token)
  assert repeat['existing'] and api(wp+'/notifications?kind=OpeningDispatch',token=token)['notifications']==notes
  other=api('/api/v1/auth/register','POST',{'username':'Other-'+secrets.token_hex(4),'password':secrets.token_urlsafe(30)})['access_token']
  assert api(wp+'/notifications?kind=OpeningDispatch',token=other)['notifications']==[]
  cfg=tmp/'config.json';cfg.write_text(json.dumps({'server':base,'world_id':world,'token':token}));cfg.chmod(0o600)
  cli=run(str(tmp/'keryx'),'notifications',env={**clean,'POLEIA_CONFIG':str(cfg)})
  assert body['title'] in cli and all(p in cli for p in body['paragraphs']),cli
  assert '"paragraphs"' not in cli and body['host_id'] not in cli,cli
  (OUT/'keryx-notifications.txt').write_text(cli+'\n')
  from playwright.sync_api import sync_playwright
  pw=sync_playwright().start();browser=pw.firefox.launch();captures=[]
  def capture(mode,width,height,token,note):
   body=note['body']
   context=browser.new_context(viewport={'width':width,'height':height})
   context.add_cookies([{'name':'poleia_token','value':token,'url':base}]);context.add_init_script('localStorage.setItem("poleia_token",'+json.dumps(token)+')')
   page=context.new_page();errors=[];page.on('pageerror',lambda e:errors.append(str(e)))
   page.goto(base+'/world/'+world+'/map',wait_until='networkidle')
   chip=page.locator('.dispatch-chip').filter(has_text=body['title'])
   if mode=='returning-mobile':
    page.evaluate("openDrawer('notif')")
    row=page.locator('#notif-body .notif-list-item').filter(has_text=body['title'])
    row.wait_for(state='visible');row.click()
   else:
    chip.wait_for(state='visible');chip.click()
   page.wait_for_function("document.getElementById('dispatch-window-overlay')?.classList.contains('open')")
   text=page.locator('#dw-body').inner_text()
   assert all(p in text for p in body['paragraphs']),text
   rect=page.locator('#dw-body').evaluate('(el)=>({width:el.clientWidth,scrollWidth:el.scrollWidth,height:el.clientHeight,scrollHeight:el.scrollHeight})')
   assert rect['scrollWidth']<=rect['width']+1 and rect['scrollHeight']<=rect['height']+1,rect
   page.screenshot(path=str(OUT/(mode+'.png')))
   page.evaluate("closeDispatchWindow();openDrawer('notif')")
   row=page.locator('#notif-body .notif-list-item').filter(has_text=body['title'])
   row.wait_for(state='visible');row.click()
   page.wait_for_function("document.getElementById('dispatch-window-overlay')?.classList.contains('open')")
   archive_text=page.locator('#dw-body').inner_text()
   assert all(p in archive_text for p in body['paragraphs']),archive_text
   captures.append({'mode':mode,'notification':note,'text':text,'archive_text':archive_text,'errors':errors,'rect':rect})
   context.close()
  for mode,width,height in [('desktop',1280,900),('mobile',390,844)]:
   if mode=='mobile':
    token=api('/api/v1/auth/register','POST',{'username':'Opening-'+secrets.token_hex(4),'password':secrets.token_urlsafe(30)})['access_token']
    api(wp+'/join','POST',{},token)
    note=api(wp+'/notifications?kind=OpeningDispatch',token=token)['notifications'][0]
   capture(mode,width,height,token,note)
  def sql(query):
   raw=subprocess.check_output(['docker','exec','-i',containers[0],'psql','-U','postgres','-d','native_proof','-qAt','-v','ON_ERROR_STOP=1'],input='BEGIN READ ONLY;'+query+';COMMIT;',text=True).strip()
   return json.loads(raw)
  natural_rise=None
  if rise_enabled:
   trials=[]; selected=None
   for attempt in range(8):
    returning_token=api('/api/v1/auth/register','POST',{'username':'Return-'+secrets.token_hex(4),'password':secrets.token_urlsafe(30)})['access_token']
    api(wp+'/join','POST',{},returning_token)
    founded=api(wp+'/founding/settle','POST',{'name':'Opening-'+secrets.token_hex(4)},returning_token)
    city=next(m for m in api(wp+'/provinces',token=returning_token) if m.get('own'))
    pp=wp+'/provinces/'+city['id']
    options=api(pp+'/placement-options',token=returning_token)
    choices={good:[(h,g) for h in options['hexes'] for g in h['goods'] if g['good_key']==good and g.get('cap',0)>g['placed'] and not h.get('held_by')] for good in ('timber','stone')}
    trials.append({'attempt':attempt,'founded':founded,'options':{k:len(v) for k,v in choices.items()}})
    if all(choices.values()):
     selected={k:max(v,key=lambda x:x[1]['marginal_yield']) for k,v in choices.items()};break
   assert selected,trials
   actions=[]
   for good,(hx,g) in selected.items():
    options=api(pp+'/placement-options',token=returning_token)
    if options['pool_size']==0:
     donor=next(p for p in api(pp+'/placements',token=returning_token)['placements'] if p['good_key']=='grain')
     api(pp+'/placements/'+str(donor['gubbe_ordinal']),'DELETE',token=returning_token)
    actions.append(api(pp+'/placements','POST',{'target_kind':'hex','hex_q':hx['hex_q'],'hex_r':hx['hex_r'],'good_key':good},returning_token))
   barracks=next(b for b in api('/api/v1/buildings') if b['type']=='barracks')
   deadline=time.monotonic()+180
   while time.monotonic()<deadline:
    goods={g['key']:g['amount'] for g in api(pp+'/goods',token=returning_token)}
    if all(goods.get(k,0)>=v for k,v in barracks['costs'].items()):break
    time.sleep(.5)
   else:raise AssertionError(('barracks production timeout',goods,barracks))
   actions.append(api(pp+'/build','POST',{'building_type':'barracks'},returning_token))
   deadline=time.monotonic()+180
   recruited=[]
   while time.monotonic()<deadline:
    try:
     response=api(pp+'/recruit','POST',{'unit_type':'spearman'},returning_token)
     recruited.append(response)
     print('natural recruit',len(recruited),flush=True)
    except urllib.error.HTTPError as e:
     error=json.loads(e.read())
     # Wait for barracks completion, a natural growth tick, or collapse.
     if e.code not in (403,422):raise
    phase=sql("SELECT row_to_json(x) FROM (SELECT pwr.status,s.population,s.state FROM settlements s JOIN player_world_records pwr ON (pwr.player_id=s.owner_id OR pwr.last_settlement_id=s.id) AND pwr.world_id=s.world_id WHERE s.id='"+city['settlement_id']+"') x")
    if phase.get('status')=='dispossessed':break
    time.sleep(.5)
   else:raise AssertionError(('natural collapse timeout',phase,recruited))
   risen=api(wp+'/rise','POST',{},returning_token)
   again=api(wp+'/rise','POST',{},returning_token)
   returning_notes=api(wp+'/notifications?kind=OpeningDispatch',token=returning_token)['notifications']
   assert len(returning_notes)==2 and again['existing'],returning_notes
   returning_note=next(n for n in returning_notes if n['body']['returning'])
   assert returning_note['body']['title']=='Begin again' and '60 silver' in ' '.join(returning_note['body']['paragraphs']),returning_note
   cfg.write_text(json.dumps({'server':base,'world_id':world,'token':returning_token}))
   returning_cli=run(str(tmp/'keryx'),'notifications',env={**clean,'POLEIA_CONFIG':str(cfg)})
   assert all(p in returning_cli for p in returning_note['body']['paragraphs']),returning_cli
   (OUT/'keryx-returning-notifications.txt').write_text(returning_cli+'\n')
   # Desktop reads the dispatch; mobile then reaches the same persistent archive.
   capture('returning-desktop',1280,900,returning_token,returning_note)
   # A read dispatch is absent from the strip, so use the archive for the second viewport.
   capture('returning-mobile',390,844,returning_token,returning_note)
   natural_rise={'trials':trials,'actions':actions,'recruited':recruited,'phase':phase,'risen':risen,'retry':again,'notifications':returning_notes}
  assert all(not c['errors'] for c in captures),captures
  (OUT/'verification.json').write_text(json.dumps({'health':health,'natural_rise':natural_rise,'notification':note,'repeat_join':repeat,'captures':captures,'sql_writes':False,'payload_overrides':False,'production_diff_sha256':diff},indent=2)+'\n')
  print('PASS natural join/retry/private archive; Keryx + native Firefox1280/390 via chip and archive')
finally:
 if browser:browser.close()
 if pw:pw.stop()
 if proc:
  proc.terminate()
  try:proc.wait(timeout=10)
  except subprocess.TimeoutExpired:proc.kill();proc.wait()
 for name in containers:subprocess.run(['docker','rm','-fv',name],capture_output=True)
