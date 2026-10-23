#!/usr/bin/env python3
"""Owned fresh runtime: normal player founding, explicit incense/temple fixture,
then native Potnia Offer, memory read and real CLI. No shared-world writes.
"""
import hashlib,json,os,pathlib,secrets,socket,subprocess,tempfile,time,urllib.request,pty,select,errno
from playwright.sync_api import sync_playwright
ROOT=pathlib.Path(__file__).resolve().parents[1]
OUT=ROOT/'docs/reviews'/os.environ.get('INCENSE_PROOF_REPORT','incense-parity')/os.environ.get('INCENSE_PROOF_RUN','run1')
WAIT_PROOF=os.environ.get('PRAYER_WAIT_PROOF')=='1'
COMPOSE_PROOF=os.environ.get('PRAYER_COMPOSE_PROOF')=='1'
COMPOSE_CLI=os.environ.get('PRAYER_COMPOSE_CLI')=='1'
INTERACTIVE=os.environ.get('PRAYER_INTERACTIVE_PROOF')=='1'
GIFT_AMOUNT=5.25 if INTERACTIVE else 5
OUT.mkdir(parents=True,exist_ok=True)
containers=[];proc=None
prefix='megaron-incense-'+secrets.token_hex(4)
def run(*args,**kw):return subprocess.check_output(args,text=True,**kw).strip()
def launch(service,image,args):
 name=prefix+'-'+service;port='5432' if service=='pg' else '6379'
 run('docker','run','-d','--rm','--name',name,'-p','127.0.0.1::'+port,*args,image);containers.append(name)
 bindings=json.loads(run('docker','inspect',name))[0]['NetworkSettings']['Ports']
 return int(bindings[port+'/tcp'][0]['HostPort'])
def terminal_command(args,env,answer):
 master,slave=pty.openpty();child=None;output=b'';answered=False
 try:
  child=subprocess.Popen(args,env=env,stdin=slave,stdout=slave,stderr=slave,start_new_session=True)
  os.close(slave);slave=None;deadline=time.monotonic()+30
  while time.monotonic()<deadline:
   ready,_,_=select.select([master],[],[],.2)
   if ready:
    try:chunk=os.read(master,65536)
    except OSError as e:
     if e.errno==errno.EIO:break
     raise
    if not chunk:break
    output+=chunk
    if b'Cast? [y/N]:' in output and not answered:
     os.write(master,(answer+'\n').encode());answered=True
   elif child.poll() is not None:break
  assert answered,'interactive prompt missing: '+output.decode(errors='replace')
  code=child.wait(timeout=5)
  assert code==0,output.decode(errors='replace')
  return output.decode(errors='replace')
 finally:
  if child is not None and child.poll() is None:child.kill();child.wait()
  os.close(master)
  if slave is not None:os.close(slave)
def main():
 global proc
 with tempfile.TemporaryDirectory(prefix='incense-live-') as tmp:
  tmp=pathlib.Path(tmp);clean={k:os.environ[k] for k in ('HOME','PATH')};clean['CGO_ENABLED']='0'
  commit=run('git','-C',str(ROOT),'rev-parse','--short=7','HEAD')+('+confirmation' if INTERACTIVE else '+compose' if COMPOSE_PROOF else '+prayerwait' if WAIT_PROOF else '+incense')
  diffhash=hashlib.sha256(subprocess.check_output(['git','-C',str(ROOT),'diff','HEAD','--','server','web'])).hexdigest()
  run('go','build','-ldflags','-X main.buildCommit='+commit,'-o',str(tmp/'temenos'),'./cmd/server',cwd=ROOT/'server',env=clean)
  run('go','build','-o',str(tmp/'keryx'),'./cmd/keryx',cwd=ROOT/'server',env=clean)
  pg=launch('pg','postgres:16-alpine',['-e','POSTGRES_PASSWORD=incense-proof','-e','POSTGRES_DB=incense_proof'])
  redis=launch('redis','redis:7-alpine',[])
  for _ in range(80):
   if subprocess.run(['docker','exec',containers[0],'pg_isready','-U','postgres','-q'],capture_output=True).returncode==0:break
   time.sleep(.25)
  with socket.socket() as sock:sock.bind(('127.0.0.1',0));port=sock.getsockname()[1]
  base=f'http://127.0.0.1:{port}'
  env={**clean,'DATABASE_URL':f'postgres://postgres:incense-proof@localhost:{pg}/incense_proof?sslmode=disable','REDIS_URL':f'localhost:{redis}',
   'JWT_SECRET':secrets.token_urlsafe(40),'PORT':str(port),'TICK_SECONDS':'3600','MAP_WIDTH':'56','MAP_HEIGHT':'40',
   'WORLD_NAME':'Incense proof','POLEIA_WORLD_START_WANAXES':'1','STATIC_DIR':str(ROOT/'web/static'),'TEMPLATE_DIR':str(ROOT/'web/templates'),
   'CHRONICLE_DIR':str(tmp/'chronicles'),'REPORTS_DIR':str(tmp/'reports')}
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
  else:raise RuntimeError('server timeout')
  latest=max(int(p.name.split('_')[0]) for p in (ROOT/'server/db/migrations').glob('*.up.sql'))
  assert health['commit']==commit and health['migration']==latest,health
  world=api('/api/v1/worlds')[0]['id'];wp='/api/v1/worlds/'+world
  token=api('/api/v1/auth/register','POST',{'username':'Incense-'+secrets.token_hex(4),'password':secrets.token_urlsafe(30)})['access_token']
  api(wp+'/join','POST',{},token)
  founded=api(wp+'/founding/settle','POST',{'name':'Incense Proof'},token)
  city=next(m for m in api(wp+'/provinces',token=token) if m.get('own'))
  sid,province=city['settlement_id'],city['id'];pid=api('/api/v1/auth/me',token=token)['id']
  def sql(q):return run('docker','exec',containers[0],'psql','-U','postgres','-d','incense_proof','-At','-v','ON_ERROR_STOP=1','-c',q)
  target=json.loads(sql(f"SELECT row_to_json(t) FROM (SELECT q,r FROM map_tiles WHERE world_id='{world}' AND terrain IN('scrub_maquis','semi_desert') AND greatest(abs(q-{city['q']}),abs(r-{city['r']}),abs(q+r-{city['q']+city['r']}))>12 ORDER BY q,r LIMIT1)t".replace('LIMIT1','LIMIT 1')))
  # Only this fresh test DB changes. The own known incense must be excluded;
  # the distant incense is the sole unknown oracle candidate.
  fixture=f"""UPDATE map_tiles SET copper_deposit=false,tin_deposit=false,silver_deposit=false,incense_deposit=false WHERE world_id='{world}';
  UPDATE map_tiles SET incense_deposit=true WHERE world_id='{world}' AND ((q={target['q']} AND r={target['r']}) OR(q={city['q']} AND r={city['r']}));
  UPDATE provinces SET incense_deposit=true WHERE id='{province}';
  INSERT INTO buildings(settlement_id,building_type,level,god_id) SELECT '{sid}','temple',1,'potnia' WHERE NOT EXISTS(SELECT1 FROM buildings WHERE settlement_id='{sid}' AND building_type='temple' AND god_id='potnia');
  UPDATE settlement_goods SET amount=1000,rate=0 WHERE settlement_id='{sid}' AND good_key IN('grain','silver');
  UPDATE player_world_records SET kharis_amount=90,kharis_rate=0,kharis_calc_tick=current_world_tick() WHERE world_id='{world}' AND player_id='{pid}';""".replace('SELECT1','SELECT 1')
  if COMPOSE_PROOF:
   fixture+=f"UPDATE settlement_goods SET amount=CASE WHEN good_key='incense' THEN 100 ELSE 0 END,rate=0,calc_tick=current_world_tick() WHERE settlement_id='{sid}';"
  sql(fixture);(OUT/'fixture.sql').write_text(fixture)
  before=next(t for t in api(wp+'/map',token=token) if (t['q'],t['r'])==(target['q'],target['r']));assert before['tier']=='fog'
  config=tmp/'config.json'
  with os.fdopen(os.open(config,os.O_WRONLY|os.O_CREAT,0o600),'w') as f:json.dump(dict(server=base,token=token,world_id=world,province_id=province),f)
  def cli(*args):return run(str(tmp/'keryx'),*args,env={**clean,'POLEIA_CONFIG':str(config)})
  cities=cli('cities');assert 'incense' in cities,cities;(OUT/'cities.txt').write_text(cities)
  prayers=cli('rite','--list');assert 'incense' in prayers,prayers;(OUT/'prayers.txt').write_text(prayers)
  if COMPOSE_PROOF:
   initial=api(wp+'/provinces/'+province,token=token)['settlement']['available_prayers']
   initial=next(p for p in initial if p['id']=='minoan_potnia_vision_v2')
   assert initial['can_prepare_offering'] and not initial['affordable'],initial
   assert 'compose' in next(l for l in prayers.splitlines() if 'minoan_potnia_vision_v2' in l)
   (OUT/'prayer-before.json').write_text(json.dumps(initial,indent=2))

  errors=[];captures=[];result=None
  with sync_playwright() as pw:
   browser=pw.firefox.launch()
   for label,w,h in [('desktop',1280,900),('mobile',390,844)]:
    ctx=browser.new_context(viewport={'width':w,'height':h},has_touch=label=='mobile')
    ctx.add_cookies([{'name':'poleia_token','value':token,'url':base}]);ctx.add_init_script('localStorage.setItem("poleia_token",'+json.dumps(token)+')')
    page=ctx.new_page();page.on('pageerror',lambda e:errors.append(str(e)))
    page.goto(base+'/world/'+world+'/map',wait_until='networkidle')
    page.locator('#trig-kult').click();page.wait_for_function("document.getElementById('kult-body')?.textContent.includes('incense')")
    for brief in page.locator('.lb-dismiss').all():
     if brief.is_visible():brief.click()
    if label=='desktop':
     open_button=page.locator("button[onclick=\"okRite('minoan_potnia_vision_v2')\"]")
     if COMPOSE_PROOF:assert open_button.inner_text().lower()=='compose offering',open_button.inner_text()
     open_button.click()
     bring=page.locator("button[onclick=\"bringOffering('minoan_potnia_vision_v2')\"]")
     if COMPOSE_PROOF:
      posts=[]
      page.on('request',lambda req:posts.append(req.url) if req.method=='POST' and req.url.endswith('/rite') else None)
      assert all(i.input_value()=='0' for i in page.locator('.offer-input').all())
      bring.click();page.get_by_text('Choose goods for your offering.',exact=True).wait_for()
      assert not posts,'empty composition dispatched an implicit traditional gift'
      page.locator('.offer-input[data-good="incense"]').fill(str(GIFT_AMOUNT))
      page.screenshot(path=str(OUT/'composition.png'))
     if COMPOSE_CLI:
      if INTERACTIVE:
       args=[str(tmp/'keryx'),'rite','--prayer','minoan_potnia_vision_v2','--offer',f'incense={GIFT_AMOUNT}']
       cli_env={**clean,'POLEIA_CONFIG':str(config),'TERM':'dumb'}
       declined=terminal_command(args,cli_env,'n');(OUT/'declined.txt').write_text(declined)
       assert 'Cast cancelled' in declined,declined
       unchanged={g['key']:g['amount'] for g in api(wp+'/provinces/'+province+'/goods',token=token)}
       assert unchanged['incense']==100 and unchanged['grain']==0 and unchanged['silver']==0,unchanged
       assert int(sql(f"SELECT count(*) FROM events WHERE world_id='{world}' AND event_type='RiteCast'"))==0
       assert f'Offer: incense×{GIFT_AMOUNT}' in declined,declined
       accepted=terminal_command(args,cli_env,'y');(OUT/'accepted.txt').write_text(accepted)
       assert f'Offer: incense×{GIFT_AMOUNT}' in accepted,accepted
       event=json.loads(sql(f"SELECT payload FROM events WHERE world_id='{world}' AND event_type='RiteCast' ORDER BY created_at DESC LIMIT1".replace('LIMIT1','LIMIT 1')))
       assert event['offering']=={'incense':GIFT_AMOUNT},event
       (OUT/'cast-event.json').write_text(json.dumps(event,indent=2))
       result={'effect':event['effect'],'message':accepted,'source':'actual RiteCast outcome + terminal stdout'}
      else:
       raw=cli('rite','--prayer','minoan_potnia_vision_v2','--offer',f'incense={GIFT_AMOUNT}','--yes','--json')
       (OUT/'cast-cli.json').write_text(raw);result=json.loads(raw)
      page.locator('#trig-kult').click();page.locator('#trig-kult').click()
     else:
      with page.expect_response(lambda r:r.request.method=='POST' and r.url.endswith('/rite')) as reply:bring.click()
      assert reply.value.status==200;result=reply.value.json()
     ore=result['effect']['revealed_ore']
     assert ore and ore['ore']=='incense' and (ore['q'],ore['r'])==(target['q'],target['r']),result
     assert 'source of incense' in result['message'],result
     if COMPOSE_PROOF:
      goods=api(wp+'/provinces/'+province+'/goods',token=token)
      amounts={g['key']:g['amount'] for g in goods}
      assert amounts['incense']==100-GIFT_AMOUNT and amounts['grain']==0 and amounts['silver']==0,amounts
      (OUT/'stocks-after.json').write_text(json.dumps(amounts,indent=2))
    if WAIT_PROOF:
     page.locator('#kult-body').get_by_text('on cooldown — 24 days',exact=True).first.wait_for()
     assert 'insufficient offering or a waiting time applies' not in page.locator('#kult-body').inner_text()
     assert page.locator("button[onclick=\"okRite('minoan_potnia_vision_v2')\"]").count()==0
    page.locator('#kult-body').get_by_text('Marks an unseen source of copper, tin, silver or incense',exact=False).scroll_into_view_if_needed()
    page.screenshot(path=str(OUT/(label+'.png')))
    assert page.locator('#kult-body').evaluate('(el)=>el.scrollWidth<=el.clientWidth'),'horizontal overflow'
    captures.append({'mode':label,'width':w,'height':h});ctx.close()
   browser.close()
  after=next(t for t in api(wp+'/map',token=token) if (t['q'],t['r'])==(target['q'],target['r']))
  assert after['tier']=='remembered' and after['incense_deposit'] is True,after
  if WAIT_PROOF:
   status=api(wp+'/provinces/'+province,token=token)
   prayer=next(p for p in status['settlement']['available_prayers'] if p['id']=='minoan_potnia_vision_v2')
   assert not prayer.get('unavailable_reason') and prayer['cooldown_remaining_game_days']==24 and not prayer['affordable'],prayer
   (OUT/'prayer-status.json').write_text(json.dumps(prayer,indent=2))
   after_cli=cli('rite','--list');line=next(l for l in after_cli.splitlines() if 'minoan_potnia_vision_v2' in l)
   assert 'cooldown' in line and '24 days remaining' in line,line
   (OUT/'prayers-after.txt').write_text(after_cli)
  assert not errors,errors
  (OUT/'proof.json').write_text(json.dumps(dict(health=health,source_diff_sha256=diffhash,world=world,target=target,before=before,after=after,result=result,cast_via="cli" if COMPOSE_CLI else "web",captures=captures,errors=errors,fixture='normal player founding; own fresh DB temple/stock/sole-unseen-incense fixture before real actions; not natural exploration'),indent=2))
  print('PASS:',health,'native Potnia incense reveal, memory only, actual CLI, Firefox1280/390')
try:main()
finally:
 if proc is not None:proc.terminate();proc.wait(timeout=20)
 for name in containers:subprocess.run(['docker','rm','-fv',name],capture_output=True)
