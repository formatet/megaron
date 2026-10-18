#!/usr/bin/env python3
"""Own PG/Redis/Continuwuity183 upgrade, natural accounts/founding, CLI/Firefox. No SQL writes."""
from pathlib import Path
import json,os,secrets,socket,subprocess,sys,time,urllib.request,urllib.error,urllib.parse
from playwright.sync_api import sync_playwright
ROOT=Path(__file__).resolve().parents[3]
OUT=Path(sys.argv[1]).resolve();OUT.mkdir(parents=True,exist_ok=True)
private=OUT/'private';private.mkdir(mode=0o700,exist_ok=True)
containers=[];proc=None;BASE=private/'baseline-src'
envbase={k:os.environ[k] for k in ('HOME','PATH')}
def cmd(*args):return subprocess.check_output(args,text=True).strip()
def service(label,image,args,port,after=()):
 name='megaron-exempt-'+secrets.token_hex(5)+'-'+label
 cmd('docker','run','-d','--rm','--name',name,'-p','127.0.0.1::'+str(port),*args,image,*after);containers.append(name)
 return int(json.loads(cmd('docker','inspect',name))[0]['NetworkSettings']['Ports'][str(port)+'/tcp'][0]['HostPort'])
def freeport():
 with socket.socket() as s:s.bind(('127.0.0.1',0));return s.getsockname()[1]
try:
 cmd('git','-C',str(ROOT),'worktree','add','--detach',str(BASE),'9c071100')
 pg=service('pg','postgres:16-alpine',['-e','POSTGRES_PASSWORD=proof','-e','POSTGRES_DB=proof'],5432)
 redis=service('redis','redis:7-alpine',[],6379)
 for _ in range(100):
  if subprocess.run(['docker','exec',containers[0],'pg_isready','-U','postgres','-q'],capture_output=True).returncode==0:break
  time.sleep(.2)
 # init only writes private fixture credentials; subsequent bootstrap uses its real Matrix.
 matrixPort=freeport();rig=ROOT/'tools/agora_acceptance/rig.py'
 subprocess.run(['python3',str(rig),'init','--dir',str(private/'matrix'),'--port',str(matrixPort)],env=envbase,check=True)
 name='megaron-exempt-'+secrets.token_hex(5)+'-matrix'
 cmd('docker','run','-d','--rm','--name',name,'--log-driver','none','-p',f'127.0.0.1:{matrixPort}:8008','--mount',f'type=bind,src={private}/matrix/config.toml,dst=/config.toml,readonly','--tmpfs','/data','megaron-agora-acceptance:26.9.1','-c','/config.toml');containers.append(name)
 for _ in range(120):
  try:urllib.request.urlopen(f'http://127.0.0.1:{matrixPort}/_matrix/client/versions',timeout=2);break
  except Exception:time.sleep(.5)
 subprocess.run(['python3',str(rig),'bootstrap','--dir',str(private/'matrix'),'--port',str(matrixPort)],env=envbase,check=True)
 matrix=json.loads((private/'matrix/secret.json').read_text())
 port=freeport();base=f'http://127.0.0.1:{port}'
 env={**envbase,'DATABASE_URL':f'postgres://postgres:proof@127.0.0.1:{pg}/proof?sslmode=disable','REDIS_URL':f'127.0.0.1:{redis}','JWT_SECRET':secrets.token_urlsafe(32),'PORT':str(port),'TICK_SECONDS':'2','MAP_WIDTH':'56','MAP_HEIGHT':'40','WORLD_NAME':'Exemption proof','POLEIA_WORLD_START_WANAXES':'1','STATIC_DIR':str(ROOT/'web/static'),'TEMPLATE_DIR':str(ROOT/'web/templates'),'CHRONICLE_DIR':str(private/'chronicles'),'REPORTS_DIR':str(private/'reports')}
 def api(path,token=None,data=None):
  r=urllib.request.Request(base+path,data=None if data is None else json.dumps(data).encode(),headers={'Content-Type':'application/json',**({'Authorization':'Bearer '+token} if token else {})})
  try:
   with urllib.request.urlopen(r,timeout=30) as res:return res.status,json.load(res)
  except urllib.error.HTTPError as e:return e.code,json.load(e)
 def start(exe,cwd=None):
  global proc
  log=(private/'server.log').open('a');proc=subprocess.Popen([str(exe)],cwd=cwd or ROOT/'server',env=env,stdout=log,stderr=log)
  for _ in range(120):
   try:
    code,h=api('/healthz')
    if code==200:return h
   except Exception:pass
   if proc.poll() is not None:raise RuntimeError('own game process exited')
   time.sleep(.25)
  raise RuntimeError('own game health timeout')
 def stop():
  global proc
  proc.terminate();proc.wait(timeout=15);proc=None
 assert not (BASE/'server/.env').exists() and not (ROOT/'server/.env').exists()
 old=cmd('git','-C',str(BASE),'rev-parse','HEAD');assert old=='9c071100'+old[8:],old
 subprocess.run(['go','build','-ldflags','-X main.buildCommit='+old,'-o',str(private/'old-server'),'./cmd/server'],cwd=BASE/'server',env=envbase,check=True)
 baseline=start(private/'old-server',BASE/'server');assert baseline['migration']==182,baseline
 accounts={}
 for kind,prefix in [('ai','gpt-'),('luna','luna-'),('human','human-')]:
  username=prefix+'proof-'+secrets.token_hex(4)
  code,registered=api('/api/v1/auth/register',data={'username':username,'password':secrets.token_urlsafe(32)});assert code==201,code
  accounts[kind]={'token':registered['access_token'],'username':username}
 stop()
 commit=cmd('git','-C',str(ROOT),'rev-parse','HEAD')
 subprocess.run(['go','build','-ldflags','-X main.buildCommit='+commit,'-o',str(private/'new-server'),'./cmd/server'],cwd=ROOT/'server',env=envbase,check=True)
 subprocess.run(['go','build','-o',str(private/'keryx'),'./cmd/keryx'],cwd=ROOT/'server',env=envbase,check=True)
 env.update(POLEIA_AGORA_URL=matrix['base_url'],POLEIA_AGORA_ACCESS_TOKEN=matrix['temenos_token'],POLEIA_AGORA_ADMIN_ROOM=matrix['room'])
 health=start(private/'new-server');assert health['commit']==commit and health['migration']==183,health
 _,worlds=api('/api/v1/worlds');world=(worlds if isinstance(worlds,list) else worlds['worlds'])[0]['id'];wp='/api/v1/worlds/'+world
 for kind,a in accounts.items():
  code,joined=api(wp+'/join',a['token'],{});assert code==201,(kind,code)
  code,founded=api(wp+'/founding/settle',a['token'],{'name':kind+'-proof'});assert code==201,(kind,code,founded)
 # Startup pass is the same production reconciler; no admin/testing HTTP endpoint.
 stop();health=start(private/'new-server')
 for _ in range(120):
  code,a=api('/api/v1/agora',accounts['human']['token'])
  if a.get('state')=='ready':break
  time.sleep(.5)
 else:raise AssertionError('ordinary human account never became ready')
 observations={};errors=[]
 for kind,a in accounts.items():
  code,status=api('/api/v1/agora',a['token']);assert code==200,(kind,code)
  want='ready' if kind=='human' else 'exempt';assert status['state']==want,(kind,status)
  observations[kind]={'status':status}
  cfg=private/(kind+'.json');cfg.write_text(json.dumps({'server':base,'token':a['token']}));cfg.chmod(0o600)
  def cli(label,*args):
   r=subprocess.run([str(private/'keryx'),*args],env={**envbase,'POLEIA_CONFIG':str(cfg)},capture_output=True,text=True)
   text=r.stdout+r.stderr;(OUT/(kind+'-'+label+'.txt')).write_text(text);return r,text
  r,text=cli('status','agora');assert r.returncode==0,text
  if kind!='human':
   assert 'No community chat account' in text and 'pending' not in text,text
   code,body=api('/api/v1/agora/password',a['token'],{});assert code==403 and 'password' not in body,(code,body)
   r,text=cli('password-denied','agora','password');assert r.returncode and 'no community chat account' in text and 'later' not in text,text
   observations[kind]['password_http']=code
  else:
   code,secret=api('/api/v1/agora/password',a['token'],{});assert code==200 and secret.get('password'),code
   login=urllib.request.Request(matrix['base_url']+'/_matrix/client/v3/login',data=json.dumps({'type':'m.login.password','identifier':{'type':'m.id.user','user':status['user_id']},'password':secret['password']}).encode(),headers={'Content-Type':'application/json'})
   with urllib.request.urlopen(login,timeout=30) as res:assert 'access_token' in json.load(res)
   secret.clear();observations[kind]['password_login']=True
  with sync_playwright() as pw:
   browser=pw.firefox.launch();page=browser.new_page(viewport={'width':1280,'height':900},device_scale_factor=1);page.on('pageerror',lambda e:errors.append(str(e)))
   page.goto(base+'/',wait_until='domcontentloaded');page.evaluate('t=>localStorage.setItem("poleia_token",t)',a['token']);page.context.add_cookies([{'name':'poleia_token','value':a['token'],'url':base}]);page.goto(base+'/play',wait_until='networkidle')
   page.wait_for_selector('#gt-wanax');page.locator('#gt-wanax').click();page.wait_for_function("!document.getElementById('acc-chat')?.hidden")
   text=page.locator('#acc-chat').inner_text();(OUT/(kind+'-web.txt')).write_text(text)
   if kind!='human':assert 'No community chat account' in text and page.locator('#acc-chat-get').count()==0,text
   else:assert page.locator('#acc-chat-get').count()==1,text
   page.locator('#acc-chat').scroll_into_view_if_needed();page.screenshot(path=str(OUT/(kind+'-firefox.png')));page.set_viewport_size({'width':390,'height':844});page.screenshot(path=str(OUT/(kind+'-mobile.png')));browser.close()
 # Read-only DB witness: flags and no AI reservation/notices despite owning cities.
 sql="SELECT username,agora_exempt,coalesce(agora_state,''),coalesce(agora_localpart,''),(SELECT count(*) FROM settlements WHERE owner_id=p.id),(SELECT count(*) FROM notifications WHERE player_id=p.id AND kind='agora_ready') FROM players p ORDER BY username"
 db=cmd('docker','exec',containers[0],'psql','-U','postgres','-d','proof','-At','-c',sql)
 (OUT/'db-readonly.txt').write_text(db+'\n');assert not errors,errors
 (OUT/'evidence.json').write_text(json.dumps({'baseline_health':baseline,'health':health,'accounts':observations,'page_errors':errors,'matrix_version':'26.9.1'},indent=2))
 print('PASS natural183 upgrade + gpt/luna no Agora/password/notice; ordinary human ready; Firefox1280/390, no pageerrors',flush=True)
finally:
 if proc:stop()
 for name in reversed(containers):subprocess.run(['docker','rm','-fv',name],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
 if BASE.exists():subprocess.run(['git','-C',str(ROOT),'worktree','remove',str(BASE)],check=True)
