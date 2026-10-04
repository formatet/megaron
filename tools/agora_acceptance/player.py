#!/usr/bin/env python3
"""Real game -> isolated Matrix -> Element acceptance; credentials never printed."""
import argparse,json,os,re,secrets,signal,subprocess,time,unicodedata,urllib.request,urllib.error,urllib.parse,uuid
from pathlib import Path
GAME='http://127.0.0.1:18097'
MATRIX='http://127.0.0.1:18100'
ELEMENT='http://127.0.0.1:18098'
MATRIX_CONTAINER='megaron-agora-clean-matrix-1'

class ProofFailure(Exception):pass

def require(condition,label):
 if not condition:raise ProofFailure(label)

def private(path,value):
 path.parent.mkdir(mode=0o700,parents=True,exist_ok=True);os.chmod(path.parent,0o700)
 fd=os.open(path,os.O_WRONLY|os.O_CREAT|os.O_TRUNC,0o600)
 with os.fdopen(fd,'w') as f:json.dump(value,f)

def request(base,method,path,data=None,token=None):
 require(base in [GAME,MATRIX],'unexpected service URL')
 r=urllib.request.Request(base+path,data=None if data is None else json.dumps(data).encode(),method=method,headers={'Content-Type':'application/json',**({'Authorization':'Bearer '+token} if token else {})})
 try:
  with urllib.request.urlopen(r,timeout=35) as x:return x.status,json.load(x),dict(x.headers)
 except urllib.error.HTTPError as e:return e.code,json.load(e),dict(e.headers)

def main():
 p=argparse.ArgumentParser(description=__doc__);p.add_argument('--dir',required=True,type=Path);p.add_argument('--matrix-fixture',required=True,type=Path);p.add_argument('--scenario',choices=['basic','collision','outage','crash'],default='basic');p.add_argument('--keryx',type=Path);p.add_argument('--game-log',type=Path);p.add_argument('--second-pass',action='store_true');a=p.parse_args()
 m=json.loads(a.matrix_fixture.read_text());require(m['base_url']==MATRIX,'expected isolated Matrix fixture')
 status,health,_=request(GAME,'GET','/healthz');require(status==200,'game health');print('Game health verified, migration',health.get('migration'),flush=True)
 token=m['temenos_token'];room=urllib.parse.quote(m['room'],safe='')
 def admin(body):
  _,sync,_=request(MATRIX,'GET','/_matrix/client/v3/sync?timeout=0',token=token);cursor=sync['next_batch']
  status,sent,_=request(MATRIX,'PUT',f'/_matrix/client/v3/rooms/{room}/send/m.room.message/{uuid.uuid4().hex}',{'msgtype':'m.text','body':body},token);require(status==200,'admin send')
  for _ in range(35):
   _,response,_=request(MATRIX,'GET','/_matrix/client/v3/sync?timeout=1000&since='+urllib.parse.quote(cursor),token=token);cursor=response['next_batch']
   for joined in response.get('rooms',{}).get('join',{}).values():
    for event in joined.get('timeline',{}).get('events',[]):
     content=event.get('content',{})
     if event.get('sender')=='@conduit:agora.test' and content.get('m.relates_to',{}).get('m.in_reply_to',{}).get('event_id')==sent['event_id']:return content.get('body','')
  raise ProofFailure('missing correlated admin reply')
 status,worlds,_=request(GAME,'GET','/api/v1/worlds');require(status==200,'world list')
 worlds=worlds if isinstance(worlds,list) else worlds['worlds'];require(len(worlds)==1,'expected one isolated world');world=worlds[0]['id']
 username='agoraproof'+secrets.token_hex(5);game_password='M'+secrets.token_urlsafe(32)
 status,registered,_=request(GAME,'POST','/api/v1/auth/register',{'username':username,'password':game_password});require(status==201,'registration');game_token=registered['access_token']
 def game(method,path,data=None):return request(GAME,method,path,data,game_token)
 status,pending,_=game('GET','/api/v1/agora');require(status==200 and pending.get('enabled') and pending.get('state')=='pending','pending before join')
 status,joined,_=game('POST',f'/api/v1/worlds/{world}/join',{'province_name':'Proof'+secrets.token_hex(3)});require(status in [200,201] and joined.get('host_unit_id'),'join creates nomadic host');join_status=status
 status,me,_=game('GET','/api/v1/auth/me');require(status==200 and me.get('wanax_name'),'wanax identity');wanax=me['wanax_name']
 localpart=re.sub('[^a-z0-9._-]','', ''.join(c for c in unicodedata.normalize('NFKD',wanax.lower()) if not unicodedata.combining(c)))
 require(bool(localpart),'normalized wanax localpart')
 status,pending,_=game('GET','/api/v1/agora');require(status==200 and pending.get('state')=='pending','pending before first city')
 fixture={'username':username,'game_password':game_password,'token':game_token,'player_id':me['id'],'world':world,'wanax_name':wanax,'server':GAME,'element_url':ELEMENT,'matrix_url':MATRIX}
 fixture_path=a.dir/(a.scenario+'-player.json');private(fixture_path,fixture);private(a.dir/'game-player-fixture.json',fixture)
 print('PASS: actual registration and join; chat pending before first city.',flush=True)
 if a.scenario=='collision':
  secret='M'+secrets.token_urlsafe(32);body=admin(f'!admin users create-user -- {localpart} {secret}')
  require(f'Created user @{localpart}:agora.test with password' in body,'collision fixture create')
  require(f'User @{localpart}:agora.test has been deactivated' in admin(f'!admin users deactivate -- @{localpart}:agora.test'),'collision fixture deactivate')
  print('Deactivated collision fixture prepared in isolated Matrix.',flush=True)
 stopped=False;trigger=False;needs_restart=False;restart_pid=None
 def pg(sql):
  owner=subprocess.check_output(['docker','inspect','--format','{{ index .Config.Labels "com.docker.compose.project" }}','megaron-agora-game-postgres-1'],text=True).strip();require(owner=='megaron-agora-game','isolated DB container ownership')
  result=subprocess.run(['docker','exec','-i','megaron-agora-game-postgres-1','psql','-U','agora_acceptance','-d','agora_acceptance','-v','ON_ERROR_STOP=1','-tA'],input=sql.encode(),capture_output=True);require(result.returncode==0,'private fixture SQL');return result.stdout.decode().strip()
 def drop_trigger():
  pg('DROP TRIGGER IF EXISTS agora_acceptance_block_ready ON players; DROP FUNCTION IF EXISTS agora_acceptance_block_ready();')
 def restart_game():
  root=Path('/tmp/agora-game-private');fd=os.open(root/'game.log',os.O_WRONLY|os.O_CREAT|os.O_APPEND,0o600)
  with os.fdopen(fd,'a') as log:
   process=subprocess.Popen(['python3',str(Path(__file__).with_name('game.py')),'run','--dir',str(root)],stdout=log,stderr=log,start_new_session=True,env={'PATH':os.environ.get('PATH','/usr/bin:/bin'),'HOME':str(root)})
  fd=os.open(root/'game.pid',os.O_WRONLY|os.O_CREAT|os.O_TRUNC,0o600);os.write(fd,str(process.pid).encode());os.close(fd)
  return process.pid
 def notices():
  status,data,_=game('GET',f'/api/v1/worlds/{world}/notifications?kind=agora_ready');require(status==200,'notification list');return data['notifications']
 def wait_ready(started):
  while time.monotonic()-started<=60:
   try:status,account,_=game('GET','/api/v1/agora')
   except (urllib.error.URLError,TimeoutError,ConnectionResetError):time.sleep(.5);continue
   if status==200 and account.get('state')=='ready':
    elapsed=time.monotonic()-started;require(elapsed<=60,'ready did not arrive within 60 seconds');return account,elapsed
   time.sleep(.5)
  raise ProofFailure('ready did not arrive within 60 seconds')
 try:
  if a.scenario=='crash':
   player_id=str(uuid.UUID(me['id']));require(pg("SELECT count(*) FROM pg_trigger WHERE tgname='agora_acceptance_block_ready'")=='0','no pre-existing crash fixture trigger')
   trigger=True
   pg("CREATE FUNCTION agora_acceptance_block_ready() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.id='"+player_id+"'::uuid AND NEW.agora_state='ready' THEN RAISE EXCEPTION 'acceptance fixture blocks ready'; END IF; RETURN NEW; END $$; CREATE TRIGGER agora_acceptance_block_ready BEFORE UPDATE OF agora_state ON players FOR EACH ROW EXECUTE FUNCTION agora_acceptance_block_ready();")
  if a.scenario=='outage':
   inspected=subprocess.check_output(['docker','inspect','--format','{{ index .Config.Labels "com.docker.compose.project" }}',MATRIX_CONTAINER],text=True).strip();require(inspected=='megaron-agora-clean','outage container ownership')
   subprocess.run(['docker','stop',MATRIX_CONTAINER],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL,check=True);stopped=True
  started=time.monotonic()
  status,founded,_=game('POST',f'/api/v1/worlds/{world}/founding/settle',{'name':'Proof'+secrets.token_hex(3)});require(status in [200,201] and founded.get('settlement_id'),'actual first city founding');settle_status=status
  fixture['settlement_id']=founded['settlement_id'];private(fixture_path,fixture);private(a.dir/'game-player-fixture.json',fixture)
  print('PASS: first city founded through actual player verb.',flush=True)
  if stopped:
   for _ in range(13):
    time.sleep(5);status,account,_=game('GET','/api/v1/agora');require(status==200 and account.get('state')!='ready','not ready during outage');require(not notices(),'no notice during outage')
   print('PASS: Matrix outage spans a reconciliation pass; no ready state or notice.',flush=True)
   started=time.monotonic()
   subprocess.run(['docker','start',MATRIX_CONTAINER],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL,check=True);stopped=False
  if a.scenario=='crash':
   while time.monotonic()-started<=90:
    row=pg("SELECT COALESCE(agora_state,''),COALESCE(agora_claim_localpart,'') FROM players WHERE id='"+player_id+"'::uuid")
    parts=row.split('|')
    if len(parts)==2 and parts[0]=='creating' and parts[1]:
     claim=parts[1];status,profile,_=request(MATRIX,'GET','/_matrix/client/v3/profile/'+urllib.parse.quote('@'+claim+':agora.test',safe='')+'/displayname')
     if status==200 and profile.get('displayname')==wanax:break
    time.sleep(.5)
   else:raise ProofFailure('remote create/display did not succeed while ready was blocked')
   require(not notices(),'no notice before crash recovery')
   pid=int(Path('/tmp/agora-game-private/game.pid').read_text());require(Path('/proc')/str(pid)/'exe' != Path('/proc/self/exe'),'own game PID')
   require((Path('/proc')/str(pid)/'exe').resolve()==Path('/tmp/agora-game-private/temenos'),'own game executable before stop')
   needs_restart=True;os.kill(pid,signal.SIGTERM)
   for _ in range(100):
    try:alive=(Path('/proc')/str(pid)/'exe').resolve()==Path('/tmp/agora-game-private/temenos')
    except (OSError,RuntimeError):alive=False
    if not alive:break
    time.sleep(.1)
   require(not alive,'own game process stopped')
   drop_trigger();trigger=False;started=time.monotonic();restart_pid=restart_game();needs_restart=False
   print('PASS: actual remote create/display survived blocked ready transaction and own game process termination.',flush=True)
  account,elapsed=wait_ready(started);require(account.get('homeserver')==MATRIX,'public local homeserver');expected=localpart+'2' if a.scenario=='collision' else localpart
  require(account.get('localpart')==expected and account.get('user_id')=='@'+expected+':agora.test','expected identity and collision suffix')
  status,profile,_=request(MATRIX,'GET','/_matrix/client/v3/profile/'+urllib.parse.quote(account['user_id'],safe='')+'/displayname');require(status==200 and profile.get('displayname')==wanax,'unchanged wanax display name')
  rows=notices();require(len(rows)==1 and rows[0]['body'].get('user_id')==account['user_id'],'exactly one correct notice')
  print('PASS: ready within %.1fs; correct Matrix ID, unchanged display name, exactly one notice.'%elapsed,flush=True)
  passwords=[]
  if a.scenario!='crash':
   for _ in range(2):
    status,value,headers=game('POST','/api/v1/agora/password',{});require(status==200 and value.get('password') and headers.get('Cache-Control')=='no-store','private password response');require(value.get('user_id')==account['user_id'],'password identity');passwords.append(value['password'])
   require(passwords[0]!=passwords[1],'password rotation')
   def matrix_login(password):return request(MATRIX,'POST','/_matrix/client/v3/login',{'type':'m.login.password','identifier':{'type':'m.id.user','user':account['user_id']},'password':password})
   status,result,_=matrix_login(passwords[0]);require(status==403 and result.get('errcode')=='M_FORBIDDEN','old Matrix password rejected')
   status,result,_=matrix_login(passwords[1]);require(status==200 and result.get('access_token'),'new Matrix password accepted')
   fixture.update({'matrix_user':account['user_id'],'matrix_old_password':passwords[0],'matrix_password':passwords[1],'agora_localpart':account['localpart']});private(fixture_path,fixture)
   subprocess.run(['python3',str(Path(__file__).with_name('element_browser.py')),'--fixture',str(fixture_path),'--old-password'],check=True)
   if a.keryx:
    cfg=a.dir/(a.scenario+'-keryx.json');private(cfg,{'server':GAME,'token':game_token,'world_id':world,'player_id':me['id'],'username':username})
    home=a.dir/'keryx-home';home.mkdir(mode=0o700,exist_ok=True)
    env={'PATH':os.environ.get('PATH','/usr/bin:/bin'),'HOME':str(home),'XDG_CONFIG_HOME':str(home/'config'),'POLEIA_CONFIG':str(cfg)}
    status=subprocess.run([str(a.keryx),'--json','agora'],env=env,capture_output=True,check=True);require(json.loads(status.stdout)['user_id']==account['user_id'],'real keryx agora status')
    listed=subprocess.run([str(a.keryx),'--json','notifications'],env=env,capture_output=True,check=True);require(b'agora_ready' in listed.stdout,'real keryx notification')
    cli=subprocess.run([str(a.keryx),'--json','agora','password'],env=env,capture_output=True,check=True);value=json.loads(cli.stdout);require(value.get('password') and value.get('user_id')==account['user_id'],'real keryx password')
    passwords.append(value['password']);require(len(set(passwords))==3,'third rotation from keryx')
    status,result,_=matrix_login(passwords[1]);require(status==403 and result.get('errcode')=='M_FORBIDDEN','keryx invalidates previous API password')
    fixture.update({'matrix_old_password':passwords[1],'matrix_password':passwords[2]});private(fixture_path,fixture)
    subprocess.run(['python3',str(Path(__file__).with_name('element_browser.py')),'--fixture',str(fixture_path),'--old-password'],check=True)
    print('PASS: actual keryx status, notification, and third password rotation through Element.',flush=True)
  if a.second_pass:
   for _ in range(13):time.sleep(5)
   require(len(notices())==1,'one notice after a second reconciliation pass');print('PASS: repeated real reconciliation produces exactly one notice.',flush=True)
  if a.scenario=='crash':
   history={};cursor=None
   for _ in range(100):
    path=f'/_matrix/client/v3/rooms/{room}/messages?dir=b&limit=100'+('&from='+urllib.parse.quote(cursor) if cursor else '')
    status,page,_=request(MATRIX,'GET',path,token=token);require(status==200,'private admin history')
    for event in page.get('chunk',[]):history[event['event_id']]=event
    end=page.get('end')
    if not page.get('chunk') or not end or end==cursor:break
    cursor=end
   creates=[e for e in history.values() if e.get('sender')=='@temenos:agora.test' and e.get('content',{}).get('body','').startswith('!admin users create-user -- '+claim+' ')]
   require(len(creates)==1,'exactly one original Matrix CREATE request after process restart')
   event_id=creates[0]['event_id'];replies=[e for e in history.values() if e.get('sender')=='@conduit:agora.test' and e.get('content',{}).get('m.relates_to',{}).get('m.in_reply_to',{}).get('event_id')==event_id and 'Created user @'+claim+':agora.test with password ' in e.get('content',{}).get('body','')]
   require(len(replies)==1,'original correlated CREATE success proves ownership')
   for event in history.values():
    if event.get('sender')!='@temenos:agora.test':continue
    body=event.get('content',{}).get('body','')
    prefixes=['!admin users create-user -- '+claim+' ','!admin users reset-password -- '+claim+' ']
    if any(body.startswith(prefix) for prefix in prefixes):
     canary=body.rsplit(' ',1)[-1];require(bool(re.fullmatch('[A-Za-z0-9_-]+',canary)),'safe provisioning canary syntax');passwords.append(canary)
   passwords=list(set(passwords));require(len(passwords)>=2,'actual create and profile-reset secret canaries')
   print('PASS: private real Matrix history contains exactly one original CREATE and its correlated success; provisioning secret canaries:',len(passwords),flush=True)
  require(bool(passwords),'nonempty secret canary set')
  # Search responses and DB projections in memory, never expose raw bodies.
  payload=json.dumps(notices());require(all(secret not in payload for secret in passwords),'no password in ready notification')
  db=subprocess.run(['docker','exec','megaron-agora-game-postgres-1','pg_dump','-U','agora_acceptance','-d','agora_acceptance','--data-only'],capture_output=True)
  require(db.returncode==0,'private whole game DB secret scan');require(all(secret.encode() not in db.stdout for secret in passwords),'no returned password anywhere in game DB')
  if a.game_log:require(all(secret not in a.game_log.read_text() for secret in passwords),'no password in game process log')
  print('PASS: no returned password anywhere in game DB'+('/log.' if a.game_log else '.'),flush=True)
 finally:
  if trigger:drop_trigger()
  if needs_restart:restart_pid=restart_game()
  if stopped:subprocess.run(['docker','start',MATRIX_CONTAINER],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL,check=True)
 proof={'scenario':a.scenario,'game_commit':health.get('commit'),'migration':health.get('migration'),'register_http':201,'join_http':join_status,'settle_http':settle_status,'agora_state':'ready','ready_seconds':round(elapsed,3),'user_id':account['user_id'],'display_name_verified':True,'agora_ready_notifications':1,'password_api_rotations':2 if a.scenario!='crash' else 0,'keryx_verified':bool(a.keryx) and a.scenario!='crash','element_old_password_rejected':a.scenario!='crash','element_current_password_home':a.scenario!='crash','actual_process_crash_recovered':a.scenario=='crash','single_matrix_create_verified':a.scenario=='crash','restarted_game_pid':restart_pid,'secret_canary_count':len(passwords),'secret_scan_whole_game_db':bool(passwords),'secret_scan_game_log':bool(a.game_log),'second_reconciliation_verified':a.second_pass,'actual_services':{'game':GAME,'matrix':MATRIX,'element':ELEMENT}}
 private(a.dir/(a.scenario+'-proof.json'),proof)
 print('COMPLETE:',a.scenario,'actual game/Matrix/Element proof; private fixture saved.',flush=True)

if __name__=='__main__':
 try:main()
 except ProofFailure as e:raise SystemExit('Acceptance failed: '+str(e)) from None
 except Exception as e:raise SystemExit('Acceptance failed: '+type(e).__name__+' (raw details withheld to protect credentials)') from None
