#!/usr/bin/env python3
"""Natural register/join -> real Host march -> actual physical founding.
Own ephemeral PostgreSQL/Redis; no SQL fixtures/writes, no live world, no API overrides.
"""
from pathlib import Path
import json,os,secrets,socket,subprocess,tempfile,time,urllib.request,urllib.error
ROOT=Path(__file__).resolve().parents[3];OUT=Path(__file__).parent/'native';OUT.mkdir(exist_ok=True)
clean={k:os.environ[k] for k in ('HOME','PATH')};clean['CGO_ENABLED']='0'
def run(*args,**kw):return subprocess.check_output(args,text=True,**kw).strip()
assert not run('git','-C',str(ROOT),'diff','HEAD','--','server','web'),'commit production before native verification'
commit=run('git','-C',str(ROOT),'rev-parse','--short=8','HEAD')
prefix='host-native-'+secrets.token_hex(5);containers=[];process=None
try:
 with tempfile.TemporaryDirectory(prefix=prefix) as tmp:
  tmp=Path(tmp)
  run('go','build','-ldflags','-X main.buildCommit='+commit,'-o',str(tmp/'server'),'./cmd/server',cwd=ROOT/'server',env=clean)
  run('go','build','-o',str(tmp/'keryx'),'./cmd/keryx',cwd=ROOT/'server',env=clean)
  def launch(service,image,port,args):
   name=prefix+'-'+service;run('docker','run','-d','--rm','--name',name,'-p','127.0.0.1::'+port,*args,image);containers.append(name)
   return run('docker','port',name,port+'/tcp').split(':')[-1]
  pg=launch('pg','postgres:16-alpine','5432',['-e','POSTGRES_PASSWORD=host-native-proof','-e','POSTGRES_DB=host_proof'])
  redis=launch('redis','redis:7-alpine','6379',[])
  for _ in range(120):
   if subprocess.run(['docker','exec',containers[0],'pg_isready','-U','postgres','-q'],capture_output=True).returncode==0:break
   time.sleep(.25)
  with socket.socket() as sock:sock.bind(('127.0.0.1',0));port=sock.getsockname()[1]
  base=f'http://127.0.0.1:{port}'
  env={**clean,'DATABASE_URL':f'postgres://postgres:host-native-proof@127.0.0.1:{pg}/host_proof?sslmode=disable','REDIS_URL':'127.0.0.1:'+redis,'JWT_SECRET':secrets.token_urlsafe(32),'PORT':str(port),'TICK_SECONDS':'3600','MAP_WIDTH':'64','MAP_HEIGHT':'64','WORLD_NAME':'Physical Host proof','POLEIA_WORLD_START_WANAXES':'1','POLEIA_REGISTRATION_CLOSED':'0','STATIC_DIR':str(ROOT/'web/static'),'TEMPLATE_DIR':str(ROOT/'web/templates'),'CHRONICLE_DIR':str(tmp/'chronicles'),'REPORTS_DIR':str(tmp/'reports')}
  with (OUT/'server.log').open('w') as log:process=subprocess.Popen([str(tmp/'server')],cwd=ROOT/'server',env=env,stdout=log,stderr=subprocess.STDOUT)
  def api(path,method='GET',data=None,token=None):
   headers={'Content-Type':'application/json'}
   if token:headers['Authorization']='Bearer '+token
   req=urllib.request.Request(base+path,method=method,data=None if data is None else json.dumps(data).encode(),headers=headers)
   try:
    with urllib.request.urlopen(req,timeout=30) as res:return res.status,json.load(res)
   except urllib.error.HTTPError as res:return res.code,json.load(res)
  for _ in range(120):
   try:
    code,health=api('/healthz')
    if code==200:break
   except (OSError,urllib.error.URLError):pass
   if process.poll() is not None:raise RuntimeError((OUT/'server.log').read_text()[-4000:])
   time.sleep(.5)
  else:raise TimeoutError('server startup')
  assert health['commit']==commit and health['migration']==198 and not health.get('migration_dirty',False),health
  code,worlds=api('/api/v1/worlds');assert code==200
  world=(worlds if isinstance(worlds,list)else worlds['worlds'])[0]['id'];wp='/api/v1/worlds/'+world
  code,auth=api('/api/v1/auth/register','POST',{'username':'Host-'+secrets.token_hex(4),'password':secrets.token_urlsafe(24)});assert code==201,(code,auth)
  token=auth['access_token']
  code,joined=api(wp+'/join','POST',{},token);assert code in (200,201),(code,joined)
  code,origin=api(wp+'/founding/status',token=token);assert code==200 and origin['active'],origin
  code,listed=api(wp+'/units',token=token);host=next(u for u in listed['units']if u['type']=='nomadic_host')
  code,tiles=api(wp+'/map',token=token);tiles=tiles if isinstance(tiles,list)else tiles['tiles']
  def distance(t):
   q=t['q']-origin['q'];r=t['r']-origin['r'];return max(abs(q),abs(r),abs(q+r))
  candidates=[t for t in tiles if distance(t)==1 and t['terrain']in('plains','river_valley','river_delta','forest_olive_grove','forest_cedar','scrub_maquis','hills')]
  assert candidates,('no adjacent valid land',origin)
  target=sorted(candidates,key=lambda t:(t['q'],t['r']))[0];q,r=target['q'],target['r']
  cfg=tmp/'config.json';cfg.write_text(json.dumps({'server':base,'world_id':world,'token':token}));cfg.chmod(0o600);cli_env={**clean,'POLEIA_CONFIG':str(cfg)}
  receipt=json.loads(run(str(tmp/'keryx'),'unit','march','--unit',host['id'],'--target',f'{q},{r}','--json',env=cli_env))
  code,current=api(wp+'/founding/status',token=token);assert code==200 and current['host_state']['status']=='marching',current
  assert (current['q'],current['r'])==(q,r)!=(origin['q'],origin['r']),(origin,current,target)
  code,listed=api(wp+'/units',token=token);moving=next(u for u in listed['units']if u['id']==host['id']);assert (moving['host_state']['q'],moving['host_state']['r'])==(q,r)
  texts={}
  for command in (('founding','status'),('status',)):
   text=run(str(tmp/'keryx'),*command,env=cli_env);assert f'Marching · physical position ({q},{r})'in text,text;texts[' '.join(command)]=text
  code,stale=api(wp+'/founding/settle','POST',{'expected_q':origin['q'],'expected_r':origin['r']},token);assert code==409,(code,stale)
  code,founded=api(wp+'/founding/settle','POST',{'name':'Physical Host Metropolis','expected_q':q,'expected_r':r},token);assert code==201,(code,founded)
  code,markers=api(wp+'/provinces',token=token);own=next(m for m in markers if m.get('own'));assert (own['q'],own['r'])==(q,r),own
  code,after=api(wp+'/founding/status',token=token);assert code==200 and after['active']is False,after
  result={'healthz':health,'natural_register_join':True,'sql_fixtures_or_writes':False,'api_overrides':False,'origin':[origin['q'],origin['r']],'target_terrain':target['terrain'],'actual_moving_position':[current['q'],current['r']],'host_state':current['host_state'],'stored_departure':[moving['q'],moving['r']],'receipt':receipt,'cli':texts,'stale_confirmation_status':409,'founding_status':201,'actual_metropolis':[own['q'],own['r']],'founder_phase_after':after}
  (OUT/'verification.json').write_text(json.dumps(result,indent=2)+'\n')
  print('PASS actual build/schema198: natural register/join -> CLI march -> physical status/API/CLI -> stale409 -> metropolis at physical target201 -> inactive phase')
finally:
 if process is not None:
  process.terminate()
  try:process.wait(timeout=10)
  except subprocess.TimeoutExpired:process.kill();process.wait()
 for name in containers:subprocess.run(['docker','rm','-fv',name],capture_output=True)
