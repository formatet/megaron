#!/usr/bin/env python3
"""Fresh real game: register → CLI help/Codex/join/found → Firefox Recruit text.
No SQL writes, FOW fabrication, replay payloads or production endpoints.
"""
from pathlib import Path
import json,os,secrets,socket,subprocess,sys,time,urllib.request
from playwright.sync_api import sync_playwright
HERE=Path(__file__).resolve().parent;ROOT=HERE.parents[2]
OUT=Path(sys.argv[1]).resolve();OUT.mkdir(parents=True,exist_ok=True)
containers=[];proc=None;cli_log=[];errors=[]
clean={k:os.environ[k] for k in ['HOME','PATH']}
def cmd(*args,**kw):return subprocess.check_output(args,text=True,**kw).strip()
def container(service,image,flags):
 name='haiku24a-'+service+'-'+secrets.token_hex(4)
 cmd('docker','run','-d','--rm','--name',name,'-p','127.0.0.1::'+('5432' if service=='pg' else '6379'),*flags,image)
 containers.append(name)
 data=json.loads(cmd('docker','inspect',name))[0]['NetworkSettings']['Ports']
 return int(next(iter(data.values()))[0]['HostPort'])
try:
 commit=cmd('git','-C',str(ROOT),'rev-parse','HEAD')
 assert not cmd('git','-C',str(ROOT),'status','--porcelain','--','server'), 'production source dirty'
 subprocess.run(['go','build','-ldflags','-X main.buildCommit='+commit,'-o',str(OUT/'temenos'),'./cmd/server'],cwd=ROOT/'server',env=clean,check=True)
 subprocess.run(['go','build','-o',str(OUT/'keryx'),'./cmd/keryx'],cwd=ROOT/'server',env=clean,check=True)
 pg=container('pg','postgres:16-alpine',['-e','POSTGRES_PASSWORD=local-proof-only','-e','POSTGRES_DB=localproof'])
 redis=container('redis','redis:7-alpine',[])
 for _ in range(60):
  if subprocess.run(['docker','exec',containers[0],'pg_isready','-U','postgres','-q'],capture_output=True).returncode==0:break
  time.sleep(.25)
 with socket.socket() as sock:sock.bind(('127.0.0.1',0));port=sock.getsockname()[1]
 base=f'http://127.0.0.1:{port}'
 env=clean|dict(DATABASE_URL=f'postgres://postgres:local-proof-only@127.0.0.1:{pg}/localproof?sslmode=disable',REDIS_URL=f'127.0.0.1:{redis}',JWT_SECRET=secrets.token_urlsafe(40),PORT=str(port),TICK_SECONDS='1',MAP_WIDTH='64',MAP_HEIGHT='64',WORLD_NAME='CLI arguments proof',POLEIA_WORLD_START_WANAXES='1',POLEIA_REGISTRATION_CLOSED='0',STATIC_DIR=str(ROOT/'web/static'),TEMPLATE_DIR=str(ROOT/'web/templates'),CHRONICLE_DIR=str(OUT/'chronicles'),REPORTS_DIR=str(OUT/'reports'))
 assert not (ROOT/'server/.env').exists()
 log=(OUT/'server.log').open('w');proc=subprocess.Popen([str(OUT/'temenos')],cwd=ROOT/'server',env=env,stdout=log,stderr=log)
 def api(path,method='GET',body=None,token=None):
  req=urllib.request.Request(base+path,method=method,data=None if body is None else json.dumps(body).encode(),headers={'Content-Type':'application/json',**({'Authorization':'Bearer '+token} if token else {})})
  with urllib.request.urlopen(req,timeout=30) as response:return json.load(response)
 for _ in range(120):
  try:health=api('/healthz');break
  except Exception:
   if proc.poll() is not None:raise RuntimeError('server exited; see server.log')
   time.sleep(.5)
 else:raise RuntimeError('server startup timeout')
 assert health['commit']==commit and health['migration']==189,health
 username='timber-'+secrets.token_hex(4);password=secrets.token_urlsafe(32)
 token=api('/api/v1/auth/register','POST',{'username':username,'password':password})['access_token']
 clienv=clean|dict(POLEIA_CONFIG=str(OUT/'private-player.json'),POLEIA_PASSWORD=password)
 def cli(*args,expect=0,binary=None):
  res=subprocess.run([str(binary or OUT/'keryx'),*args],env=clienv,capture_output=True,text=True,input="")
  cli_log.append(dict(args=args,variant="baseline" if binary else "branch",exit_code=res.returncode,stdout=res.stdout,stderr=res.stderr))
  assert res.returncode==expect,(args,res.stdout,res.stderr,expect)
  return res.stdout
 cli('login','--server',base,'--username',username)
 baseline=Path(sys.argv[2]).resolve()
 cli('join');cli('founding','settle','--name','Myrsine','--yes')
 data=json.loads((OUT/'private-player.json').read_text());wid=data['world_id'];pid=data['province_id'];world='/api/v1/worlds/'+wid
 for args in [('report','a report with spaces'),('build','harbour'),('staff',)]:
  cli(*args,expect=0 if args[0]=='build' else 1,binary=baseline)
  cli(*args,expect=1)
 cli('report','--kind','confused','--text','isolated acceptance report with spaces')
 cli('staff','--help');cli('unit','march','--help');cli('codex','keryx');cli('codex','transfers')
 units=api(world+'/units',token=token)['units']
 escort=next(u for u in units if u['category']=='land' and u['status']=='garrison' and u['size']>=100 and u['type']!='nomadic_host')
 options=api(world+'/provinces/'+pid+'/placement-options',token=token)
 center=api(world+'/provinces/'+pid,token=token)['map_tile']
 def dist(q,r):return max(abs(q-center['Q']),abs(r-center['R']),abs(q-center['Q']+r-center['R']))
 land=['plains','river_valley','hills','scrub_maquis','forest_olive_grove','forest_cedar']
 edge=max((h for h in options['hexes'] if h['terrain'] in land),key=lambda h:dist(h['hex_q'],h['hex_r']))
 cli('unit','march','--unit',escort['id'],'--target',str(edge['hex_q'])+','+str(edge['hex_r']))
 for _ in range(60):
  moved=next(u for u in api(world+'/units',token=token)['units'] if u['id']==escort['id'])
  if moved['status']=='positioned':break
  time.sleep(.5)
 assert moved['status']=='positioned','first scouting march did not arrive'
 known=api(world+'/map',token=token)
 tile=min((h for h in known if h.get('terrain') in land and dist(h['q'],h['r'])>=3),key=lambda h:dist(h['q'],h['r']))
 target={'hex_q':tile['q'],'hex_r':tile['r']}
 args=('unit','march','--unit',escort['id'],'--target',str(target['hex_q'])+','+str(target['hex_r']),'--intent','colonize','--name','West')
 preview=cli(*args);assert 'Nothing sent' in preview and '--yes' in preview, preview
 unchanged=next(u for u in api(world+'/units',token=token)['units'] if u['id']==escort['id']);assert unchanged['status']=='positioned'
 cli(*args,'--yes')
 colony=None
 for _ in range(60):
  own=api(world+'/provinces',token=token)
  colony=next((m for m in own if m.get('own') and m.get('name')=='West'),None)
  if colony:break
  time.sleep(.5)
 assert colony,'colony did not arrive naturally'
 # A client configuration is player input; only selected ProvinceID changes.
 # The colony and its inventory were created by the real march above.
 data=json.loads((OUT/'private-player.json').read_text());data['province_id']=colony['id'];(OUT/'private-player.json').write_text(json.dumps(data))
 transfer=('transfer','--good','grain','--qty','1','--dest','West')
 cli(*transfer,expect=1,binary=baseline)
 before_error=cli_log[-1]['stderr'];assert 'no own or contacted city' in before_error
 cli(*transfer,expect=1)
 after_error=cli_log[-1]['stderr'];assert 'sending city' in after_error and '--from' in after_error
 cli(*transfer,'--from','Myrsine')
 assert json.loads((OUT/'private-player.json').read_text())['province_id']==colony['id']
 with sync_playwright() as p:
  b=p.firefox.launch();page=b.new_page(viewport={'width':1280,'height':900},device_scale_factor=1)
  page.on('pageerror',lambda error:errors.append(str(error)))
  page.goto(base+'/',wait_until='domcontentloaded');page.evaluate('t=>localStorage.setItem("poleia_token",t)',token)
  page.context.add_cookies([{'name':'poleia_token','value':token,'url':base}]);page.goto(base+'/play',wait_until='networkidle')
  page.wait_for_function('window.openDrawer !== undefined');page.evaluate("window.openDrawer('economy')")
  page.wait_for_selector('#economy-body [data-tab="transfer"]');page.locator('#economy-body [data-tab="transfer"]').click()
  page.wait_for_selector('#ec-tr-from')
  page.select_option('#ec-tr-from',colony['id'])
  page.wait_for_function("id=>{const e=document.getElementById('ec-tr-to');return e&&e.querySelector('option[value=\"'+id+'\"]')}",arg=next(m['settlement_id'] for m in own if m.get('own') and m['id']==pid))
  assert not page.locator('#ec-tr-to option[value="'+colony['settlement_id']+'"]').count()
  for brief in page.locator('.lb-dismiss').all():
   if brief.is_visible():brief.click()
  for label,width,height in [('desktop',1280,900),('mobile',390,844)]:
   page.set_viewport_size({'width':width,'height':height});page.locator('#drawer-economy').screenshot(path=str(OUT/('transfer-'+label+'.png')))
  b.close()
 assert not errors,errors
 proof=dict(health=health,natural_register_cli_login_join_found_colonize=True,world_id=wid,province_id=pid,colony_id=colony['id'],selected_city_setup='Only local client province_id set to the naturally founded colony, reproducing the reported selected-colony configuration',before_error=before_error,after_error=after_error,explicit_from_succeeded=True,unconfirmed_colonize_sent_no_order=True,web_excludes_source_from_destinations=True,firefox_errors=errors,sql_writes=False,request_replay=False,browser='Firefox DPR1 1280/390',tick_seconds=1)
 (OUT/'proof.json').write_text(json.dumps(proof,indent=2)+'\n');(OUT/'cli.json').write_text(json.dumps(cli_log,indent=2)+'\n');print(json.dumps(proof))
finally:
 (OUT/'cli-attempt.json').write_text(json.dumps(cli_log,indent=2)+'\n')
 if proc is not None:proc.terminate();proc.wait(timeout=15)
 for name in reversed(containers):subprocess.run(['docker','rm','-f',name],capture_output=True)
