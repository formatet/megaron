#!/usr/bin/env python3
"""Owned fresh runtime, normal two-Wanax founding, outbound fixture delivered by
actual Arrival worker. Saved102 window, native same-bearer reply, CLI and cutoff.
Usage: python3 tools/return_post_live.py OUT [BUILD_COMMIT]
Never touches shared acceptance/live services. Explicit fixtures are recorded.
"""
from pathlib import Path
import hashlib,json,os,secrets,socket,subprocess,sys,tempfile,time,urllib.request,uuid
from playwright.sync_api import sync_playwright
ROOT=Path(__file__).resolve().parents[1]

def main():
 out=Path(sys.argv[1]).resolve();out.mkdir(parents=True,exist_ok=True)
 containers=[];proc=None;prefix='megaron-return-post-'+secrets.token_hex(4)
 def run(*args,**kw):return subprocess.check_output(args,text=True,**kw).strip()
 def launch(kind,image,args):
  name=prefix+'-'+kind;internal='5432' if kind=='pg' else '6379'
  run('docker','run','-d','--rm','--name',name,'-p','127.0.0.1::'+internal,*args,image);containers.append(name)
  ports=json.loads(run('docker','inspect',name))[0]['NetworkSettings']['Ports'];return int(ports[internal+'/tcp'][0]['HostPort'])
 try:
  with tempfile.TemporaryDirectory(prefix='return-post-proof-') as directory:
   tmp=Path(directory);clean={k:os.environ[k] for k in ('HOME','PATH')};clean['CGO_ENABLED']='0'
   commit=sys.argv[2] if len(sys.argv)>2 else run('git','-C',str(ROOT),'rev-parse','--short=7','HEAD')+'+returnpost'
   hashes={str(p.relative_to(ROOT)):hashlib.sha256(p.read_bytes()).hexdigest() for d in ('server/api/handlers','server/internal/messenger','server/cmd/keryx','web/static/js/megaron/ui/drawers') for p in (ROOT/d).glob('*') if p.is_file() and ('messenger' in p.name or 'reply' in p.name or 'handler' in p.name or 'diplomacy' in p.name or 'correspondence' in p.name)}
   run('go','build','-ldflags','-X main.buildCommit='+commit,'-o',str(tmp/'temenos'),'./cmd/server',cwd=ROOT/'server',env=clean)
   run('go','build','-o',str(tmp/'keryx'),'./cmd/keryx',cwd=ROOT/'server',env=clean)
   print('BUILD_COMPLETE',commit,flush=True)
   pg=launch('pg','postgres:16-alpine',['-e','POSTGRES_PASSWORD=returnpost','-e','POSTGRES_DB=return_post'])
   redis=launch('redis','redis:7-alpine',[])
   for _ in range(100):
    if subprocess.run(['docker','exec',containers[0],'pg_isready','-h','127.0.0.1','-U','postgres','-q'],capture_output=True).returncode==0:break
    time.sleep(.2)
   with socket.socket() as sock:sock.bind(('127.0.0.1',0));port=sock.getsockname()[1]
   base=f'http://127.0.0.1:{port}'
   env={**clean,'DATABASE_URL':f'postgres://postgres:returnpost@localhost:{pg}/return_post?sslmode=disable','REDIS_URL':f'localhost:{redis}','JWT_SECRET':secrets.token_urlsafe(40),'PORT':str(port),'TICK_SECONDS':'3600','MAP_WIDTH':'56','MAP_HEIGHT':'40','WORLD_NAME':'Return post proof','POLEIA_WORLD_START_WANAXES':'1','STATIC_DIR':str(ROOT/'web/static'),'TEMPLATE_DIR':str(ROOT/'web/templates'),'CHRONICLE_DIR':str(tmp/'chronicles'),'REPORTS_DIR':str(tmp/'reports')}
   assert not (ROOT/'server/.env').exists()
   proc=subprocess.Popen([str(tmp/'temenos')],cwd=ROOT/'server',env=env,stdout=(out/'server.log').open('w'),stderr=subprocess.STDOUT)
   def api(path,method='GET',data=None,token=None):
    req=urllib.request.Request(base+path,method=method,data=None if data is None else json.dumps(data).encode(),headers={'Content-Type':'application/json',**({'Authorization':'Bearer '+token} if token else {})});return json.load(urllib.request.urlopen(req,timeout=30))
   for _ in range(120):
    try:health=api('/healthz');break
    except Exception:
     if proc.poll() is not None:raise RuntimeError('server exited; see server.log')
     time.sleep(.5)
   else:raise RuntimeError('server timeout')
   latest=max(int(p.name.split('_')[0]) for p in (ROOT/'server/db/migrations').glob('*.up.sql'))
   assert health['commit']==commit and health['migration']==latest,health
   world=api('/api/v1/worlds')[0]['id'];wp='/api/v1/worlds/'+world
   def sql(q):return run('docker','exec',containers[0],'psql','-U','postgres','-d','return_post','-At','-v','ON_ERROR_STOP=1','-c',q)
   def actor():
    token=api('/api/v1/auth/register','POST',{'username':'ReturnPost-'+secrets.token_hex(4),'password':secrets.token_urlsafe(30)})['access_token'];api(wp+'/join','POST',{},token)
    host=next(u for u in api(wp+'/units',token=token)['units'] if u['type']=='nomadic_host')
    land=sql(f"SELECT landmass_id FROM map_tiles WHERE world_id='{world}' AND q={host['q']} AND r={host['r']}")
    return {'token':token,'host':host,'land':land,'pid':api('/api/v1/auth/me',token=token)['id']}
   recipient=actor()
   for _ in range(40):
    sender=actor()
    if sender['land']==recipient['land']:break
   else:raise AssertionError('no ordinary pair on connected landmass')
   for who,name in [(recipient,'Return Nostos'),(sender,'Return Kyme')]:
    who['sid']=api(wp+'/founding/settle','POST',{'name':name},who['token'])['settlement_id'];who['province']=next(p for p in api(wp+'/provinces',token=who['token']) if p.get('settlement_id')==who['sid'])
   ids={'reply':str(uuid.uuid4()),'closed':str(uuid.uuid4())}
   fixture=[f"UPDATE worlds SET current_tick=100,last_tick_at=now() WHERE id='{world}';",f"UPDATE settlement_goods SET amount=10000,rate=0,calc_tick=100 WHERE settlement_id IN('{recipient['sid']}','{sender['sid']}') AND good_key='grain';"]
   for name,ident in ids.items():
    scheduled_due=90 if name=='closed' else 100
    fixture += [f"INSERT INTO messengers(id,world_id,sender_id,origin_id,destination_id,message_text,status,hex_q,hex_r,sent_at,arrives_at) VALUES('{ident}','{world}','{sender['pid']}','{sender['sid']}','{recipient['sid']}','Return post {name} fixture','outbound',{recipient['province']['q']},{recipient['province']['r']},now(),now());",f"INSERT INTO scheduled_events(world_id,event_type,payload,process_after,due_tick,priority) VALUES('{world}','MessengerArrival','{{\"messenger_id\":\"{ident}\"}}',now(),{scheduled_due},0);"]
   fixture='BEGIN;\n'+'\n'.join(fixture)+'\nCOMMIT;';sql(fixture);(out/'fixture.sql').write_text(fixture)
   deadline=time.monotonic()+40
   while time.monotonic()<deadline:
    if int(sql(f"SELECT count(*) FROM messengers WHERE id IN('{ids['reply']}','{ids['closed']}') AND status='delivered'"))==2:break
    time.sleep(.2)
   else:raise AssertionError('actual Arrival worker did not deliver fixtures')
   saved=json.loads(sql(f"SELECT json_agg(t) FROM(SELECT m.id,m.status,se.event_type,se.due_tick FROM messengers m JOIN scheduled_events se ON se.payload->>'messenger_id'=m.id::text WHERE m.id IN('{ids['reply']}','{ids['closed']}') AND se.event_type='MessengerStayEnd')t"))
   assert len(saved)==2 and all(x['due_tick']==102 for x in saved),saved
   assert int(sql(f"SELECT count(*) FROM events WHERE world_id='{world}' AND event_type='MessengerArrived'"))==2
   (out/'actual-arrival.json').write_text(json.dumps(saved,indent=2))
   initial=api(wp+'/messengers/queue?limit=100',token=recipient['token']);assert len(initial['items'])==2
   assert all(m['reply_deadline_tick']==102 and not m['reply_window_closed'] and 'reply' in m['actions'] for m in initial['items'])
   (out/'queue-before.json').write_text(json.dumps(initial,indent=2))
   config=tmp/'config.json'
   with os.fdopen(os.open(config,os.O_WRONLY|os.O_CREAT,0o600),'w') as f:json.dump({'server':base,'token':recipient['token'],'world_id':world,'province_id':recipient['province']['id']},f)
   def cli(*args):return run(str(tmp/'keryx'),*args,env={**clean,'POLEIA_CONFIG':str(config)})
   before=cli('inbox','--queue');assert 'Return post before' in before and f'reply --id {ids["reply"]}' in before;assert 'day 102' not in before;(out/'cli-before.txt').write_text(before)
   errors=[];posts=[]
   with sync_playwright() as pw:
    browser=pw.firefox.launch()
    def context(width,height):
     ctx=browser.new_context(viewport={'width':width,'height':height},has_touch=width==390);ctx.add_cookies([{'name':'poleia_token','value':recipient['token'],'url':base}]);ctx.add_init_script('localStorage.setItem("poleia_token",'+json.dumps(recipient['token'])+')');return ctx
    desktop=context(1280,900);page=desktop.new_page();page.on('pageerror',lambda e:errors.append(str(e)));page.on('request',lambda r:posts.append({'method':r.method,'path':r.url.split(base)[-1]}) if r.method=='POST' else None)
    page.goto(base+'/world/'+world+'/map',wait_until='networkidle');page.locator('#trig-diplomacy').click();root=page.locator('#dtab-queue');letter=root.locator('[data-letter="'+ids['reply']+'"]');letter.wait_for();assert 'Return post before' in letter.inner_text()
    letter.locator('summary').click();letter.locator('[data-queue-reply]').fill('Carry this answer home on your own road.');page.screenshot(path=str(out/'desktop-window.png'))
    with page.expect_response(lambda r:r.request.method=='POST' and r.url.endswith('/'+ids['reply']+'/reply')) as response:letter.locator('[data-queue-action="reply"]').click()
    assert response.value.status==200,response.value.text();letter.get_by_text('Reply dispatched.',exact=True).wait_for();page.screenshot(path=str(out/'desktop-replied.png'))
    assert int(sql(f"SELECT count(*) FROM messengers WHERE world_id='{world}'"))==2,'reply created a new messenger'
    ground=json.loads(sql(f"SELECT row_to_json(t) FROM(SELECT id,status,reply_text FROM messengers WHERE id='{ids['reply']}')t"));assert ground['status']=='returning' and ground['reply_text']=='Carry this answer home on your own road.',ground
    assert int(sql(f"SELECT count(*) FROM scheduled_events WHERE world_id='{world}' AND event_type='MessengerReturn' AND payload->>'messenger_id'='{ids['reply']}'"))==1
    assert not [p for p in posts if p['path'].endswith('/messengers')],posts
    mobile=context(390,844);phone=mobile.new_page();phone.on('pageerror',lambda e:errors.append(str(e)));phone.goto(base+'/world/'+world+'/map',wait_until='networkidle');phone.locator('#trig-diplomacy').click();mroot=phone.locator('#dtab-queue');pending=mroot.locator('[data-letter="'+ids['closed']+'"]');pending.wait_for();assert 'Return post before' in pending.inner_text();phone.screenshot(path=str(out/'mobile-window.png'))
    sql(f"UPDATE worlds SET current_tick=102,last_tick_at=now() WHERE id='{world}';")
    mroot.locator('[data-queue-refresh]').click();phone.wait_for_function("document.querySelector('#dtab-queue')?.textContent.includes('Return post closed')");closed=mroot.locator('[data-letter="'+ids['closed']+'"]');assert closed.locator('[data-queue-action="reply"]').count()==0;closed.scroll_into_view_if_needed();phone.screenshot(path=str(out/'mobile-closed.png'))
    assert mroot.evaluate('(el)=>el.scrollWidth<=el.clientWidth')
    phone.locator('[data-tab="threads"]').click();phone.locator('#dtab-threads .dip-thread').first.wait_for();assert phone.locator('#dip-reply-row-'+ids['closed']).count()==0
    mobile.close();desktop.close();browser.close()
   assert not errors,errors
   after=api(wp+'/messengers/queue?limit=100',token=recipient['token']);assert all(m['reply_window_closed'] and 'reply' not in m['actions'] for m in after['items']);(out/'queue-after.json').write_text(json.dumps(after,indent=2))
   for args,name in [(('inbox','--queue'),'cli-closed.txt'),(('inbox','--history'),'cli-history.txt')]:
    text=cli(*args);assert f'reply --id {ids["closed"]}' not in text;(out/name).write_text(text)
   (out/'proof.json').write_text(json.dumps({'health':health,'world':world,'ids':ids,'saved_stays':saved,'reply_ground':ground,'posts':posts,'errors':errors,'source_hashes':hashes,'fixture':'normal register/join/found; 2 explicit outbound messengers+scheduled100/90 Arrival events; actual worker delivers at100 and saves102; tick102 advanced explicitly for cutoff view','result':'PASS'},indent=2))
   print('PASS',health,'actual Arrival100→saved102, same-bearer native reply, no new messenger, CLIdeadline+history, Firefox1280/390 closedwindow')
 finally:
  if proc is not None:proc.terminate();proc.wait(timeout=20)
  for name in containers:subprocess.run(['docker','rm','-fv',name],capture_output=True)

if __name__=='__main__':main()
