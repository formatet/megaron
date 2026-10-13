#!/usr/bin/env python3
"""Natural coastal register/join/found; actual march preview API, CLI and Firefox.
No SQL fixture/writes, request interception, supplied payloads or CSS overrides.
"""
import hashlib,json,os,pathlib,secrets,socket,subprocess,tempfile,time,urllib.request
HERE=pathlib.Path(__file__).resolve().parent;ROOT=HERE.parents[2]
OUT=HERE/os.environ.get('MARCH_PROOF_OUT','live');OUT.mkdir(parents=True,exist_ok=True)
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
   if subprocess.run(['docker','exec',containers[0],'pg_isready','-U','postgres','-q'],capture_output=True).returncode==0:break
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
  assert health['commit']==commit and health['migration']==178,health
  worlds=api('/api/v1/worlds');world=(worlds if isinstance(worlds,list) else worlds['worlds'])[0]['id'];wp='/api/v1/worlds/'+world
  token=api('/api/v1/auth/register','POST',{'username':'Spear-'+secrets.token_hex(4),'password':secrets.token_urlsafe(30)})['access_token']
  api(wp+'/join','POST',{},token)
  # Find a real coastal newcomer with its real support galley, without fixtures.
  for attempt in range(20):
   starter=api(wp+'/units',token=token)['units']
   if any(u['type']=='galley' for u in starter):break
   token=api('/api/v1/auth/register','POST',{'username':'Coast-'+secrets.token_hex(4),'password':secrets.token_urlsafe(30)})['access_token']
   api(wp+'/join','POST',{},token)
  else:raise AssertionError('no natural coastal starter in 20 joins')
  founder=api(wp+'/founding/status',token=token)
  assert founder['silver']['amount']==480 and founder['silver']['rate_per_tick']==0 and 'ticks_left' not in founder['silver'],founder
  before=api(wp+'/units',token=token)['units']
  assert all(u['upkeep_context']=='subsistence' and u['upkeep_silver_per_tick']==0 for u in before if u['type']=='spearman'),before
  cfg=tmp/'config.json';cfg.write_text(json.dumps({'server':base,'world_id':world,'token':token}));cfg.chmod(0o600)
  cli_env={**clean,'POLEIA_CONFIG':str(cfg)}
  before_cli=run(str(tmp/'keryx'),'unit','list',env=cli_env)
  assert 'Upkeep after settling: 1 grain, 0 silver per day' in before_cli,before_cli
  (OUT/'keryx-founder.txt').write_text(before_cli+'\n')
  founded=api(wp+'/founding/settle','POST',{'name':'Phaistos'},token)
  markers=api(wp+'/provinces',token=token);city=next(m for m in markers if m.get('own'));province=city['id']
  cfg.write_text(json.dumps({'server':base,'world_id':world,'province_id':province,'token':token}));cfg.chmod(0o600)
  goods=api(wp+'/provinces/'+province+'/goods',token=token);silver=next(g for g in goods if g['key']=='silver')
  assert silver['amount']==(1278 if founded.get('coastal') else 730),silver
  units=api(wp+'/units',token=token)['units'];spears=[u for u in units if u['type']=='spearman']
  assert len(spears)==2 and all(u['upkeep_grain_per_tick']==.5 and u['upkeep_silver_per_tick']==0 and u['upkeep_context']=='active' for u in spears),spears
  cli=run(str(tmp/'keryx'),'unit','list',env=cli_env);assert cli.count('Upkeep: 0.5 grain, 0 silver per day')==2,cli
  (OUT/'keryx-units.txt').write_text(cli+'\n')
  from playwright.sync_api import sync_playwright
  ship=next(u for u in units if u['type']=='galley')
  target={'q':32,'r':32,'known':False,'name':'Explore','isSea':False,'isSettlement':False}
  q,r=target['q'],target['r']
  uri=wp+'/units/'+ship['id']+f'/march-preview?target_q={q}&target_r={r}&intent=explore'
  expected=api(uri,token=token)
  assert abs(expected['upkeep_silver_home']-.45)<1e-9 and expected['upkeep_silver_field']==1.5,expected
  assert expected['silver_runway_ticks']==1278/1.5,expected
  cli=run(str(tmp/'keryx'),'unit','march','--unit',ship['id'],'--q',str(q),'--r',str(r),'--intent','explore','--preview',env=cli_env)
  assert 'Sold: 0.45 → 1.5 silver/day in the field · treasury lasts 852 days' in cli,cli
  (OUT/'keryx-preview.txt').write_text(cli+'\n')
  before_preview=api(wp+'/units',token=token)
  before_goods=api(wp+'/provinces/'+province+'/goods',token=token)
  pw=sync_playwright().start();browser=pw.firefox.launch();captures=[]
  for mode,width,height in [('desktop',1280,900),('mobile',390,844)]:
   ctx=browser.new_context(viewport={'width':width,'height':height})
   ctx.add_cookies([{'name':'poleia_token','value':token,'url':base}]);ctx.add_init_script('localStorage.setItem("poleia_token",'+json.dumps(token)+')')
   page=ctx.new_page();errors=[];page.on('pageerror',lambda e:errors.append(str(e)))
   page.goto(base+'/world/'+world+'/map',wait_until='networkidle')
   if page.locator('#dc-dismiss-all').is_visible():page.locator('#dc-dismiss-all').click()
   for brief in page.locator('.drawer.open .lb-dismiss').all():
    if brief.is_visible():brief.click()
   page.evaluate('async dest=>await openMarchCtx(dest,8,80)',target)
   page.locator('#mg-0').wait_for()
   # Ordinary form inputs choose every eligible group; no payload/CSS override.
   for choice in page.locator('#mctx-units .mctx-input').all():choice.fill(choice.get_attribute('max'))
   row=page.locator('#mctx-eta .march-sold');row.wait_for()
   page.wait_for_function("document.querySelector('#mctx-eta .march-sold')?.textContent.includes('0.45 → 1.5')")
   assert page.locator('#mctx-eta .march-sold').count()==1
   text=row.inner_text();assert text=='Sold: 0.45 → 1.5 silver/day in the field · treasury lasts 852 days',text
   row.scroll_into_view_if_needed();rect=row.bounding_box()
   assert rect and rect['x']>=0 and rect['x']+rect['width']<=width+1,rect
   assert row.evaluate('(el)=>el.scrollWidth<=el.clientWidth+1')
   assert page.locator('#mctx-send').is_visible()
   page.screenshot(path=str(OUT/(mode+'.png')))
   (OUT/(mode+'-preview.txt')).write_text(page.locator('#march-ctx').inner_text()+'\n')
   assert not errors,errors
   captures.append({'mode':mode,'viewport':[width,height],'text':text,'rect':rect,'page_errors':errors});ctx.close()
  assert api(wp+'/units',token=token)==before_preview,'preview changed units'
  assert api(wp+'/provinces/'+province+'/goods',token=token)==before_goods,'preview changed goods'
  (OUT/'verification.json').write_text(json.dumps({'health':health,'production_diff_sha256':diff,'natural_register_join_founding':True,'sql_fixtures_or_writes':False,'request_overrides':False,'first_founding':founded,'first_silver':silver,'units':units,'preview':expected,'captures':captures,'read_only_units_and_goods':True},indent=2)+'\n')
  print('PASS natural coastal founder → paid galley .45/1.5 → CLI + Firefox1280/390 single sold row; units/goods read-only')

finally:
 if browser:browser.close()
 if pw:pw.stop()
 if proc:
  proc.terminate()
  try:proc.wait(timeout=10)
  except subprocess.TimeoutExpired:proc.kill();proc.wait()
 for name in containers:subprocess.run(['docker','rm','-fv',name],capture_output=True)
