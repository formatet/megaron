#!/usr/bin/env python3
"""Read an existing PRIVATE pg_dump; independent PG/Redis restore for master
and fix. HTTP place stone #8 as the cloned Arete with a locally signed JWT.
Never connects to production or reads production credentials.
Usage: snapshot_repro.py private.dump private-output baseline-worktree
Public output: snapshot-repro.json only; server logs and binaries stay private.
"""
from pathlib import Path
import base64,hashlib,hmac,json,os,secrets,socket,subprocess,sys,time,urllib.request,urllib.error
ROOT=Path(__file__).resolve().parents[3];dump=Path(sys.argv[1]).resolve();out=Path(sys.argv[2]).resolve();out.mkdir(parents=True,exist_ok=True);os.chmod(out,0o700)
baseline=Path(sys.argv[3]).resolve();results=[]
def run(args,**kw):
 r=subprocess.run(args,capture_output=True,**kw)
 if r.returncode:
  raw=r.stderr.encode() if isinstance(r.stderr,str) else r.stderr
  (out/'private-failure.stderr').write_bytes(raw or b'')
  raise RuntimeError('private subprocess failed; inspect private-failure.stderr')
 return r
def freeport():
 with socket.socket() as sock:sock.bind(('127.0.0.1',0));return sock.getsockname()[1]
def b64(raw):return base64.urlsafe_b64encode(raw).decode().rstrip('=')
for label,source in [('master',baseline),('fix',ROOT)]:
 containers=[];proc=None;log=None
 def service(kind,img,args,port):
  name='eyes-repro-'+secrets.token_hex(5)+'-'+kind
  run(['docker','run','-d','--rm','--name',name,'-p','127.0.0.1::'+port,*args,img]);containers.append(name)
  return json.loads(run(['docker','inspect',name],text=True).stdout)[0]['NetworkSettings']['Ports'][port+'/tcp'][0]['HostPort']
 def sql(query):return run(['docker','exec',containers[0],'psql','-U','postgres','-d','snapshot','-Atc',query],text=True).stdout.strip()
 try:
  pg=service('pg','postgres:16-alpine',['-e','POSTGRES_PASSWORD=private-local','-e','POSTGRES_DB=snapshot'],'5432')
  redis=service('redis','redis:7-alpine',[],'6379')
  for _ in range(100):
   if subprocess.run(['docker','exec',containers[0],'pg_isready','-h','127.0.0.1','-U','postgres','-q'],capture_output=True).returncode==0:break
   time.sleep(.2)
  with dump.open('rb') as f:run(['docker','exec','-i',containers[0],'pg_restore','-U','postgres','-d','snapshot','--no-owner','--exit-on-error'],stdin=f)
  row=json.loads(sql("SELECT json_build_object('pid',p.id,'province_id',s.province_id,'world_id',s.world_id,'population',s.population,'tick',w.current_tick,'marching_units',(SELECT count(*) FROM units u WHERE u.owner_id=p.id AND u.status='marching')) FROM players p JOIN settlements s ON s.owner_id=p.id JOIN worlds w ON w.id=s.world_id WHERE p.username='gpt-arete' AND w.status='active' LIMIT 1"))
  assert row['marching_units']>0,row
  commit=run(['git','-C',str(source),'rev-parse','HEAD'],text=True).stdout.strip()
  trace=' + local println(err) at hex lock' if label=='master' else ''
  binary=out/(label+'-temenos');run(['go','build','-ldflags','-X main.buildCommit='+commit,'-o',str(binary),'./cmd/server'],cwd=source/'server')
  secret=secrets.token_urlsafe(32);now=int(time.time())
  unsigned=b64(json.dumps({'alg':'HS256','typ':'JWT'}).encode())+'.'+b64(json.dumps({'pid':row['pid'],'username':'gpt-arete','sub':row['pid'],'iat':now,'exp':now+3600}).encode())
  token=unsigned+'.'+b64(hmac.new(secret.encode(),unsigned.encode(),hashlib.sha256).digest())
  port=freeport();env={k:os.environ[k] for k in ('HOME','PATH')}
  env.update(DATABASE_URL=f'postgres://postgres:private-local@127.0.0.1:{pg}/snapshot?sslmode=disable',REDIS_URL='127.0.0.1:'+redis,JWT_SECRET=secret,PORT=str(port),TICK_SECONDS='3600',STATIC_DIR=str(source/'web/static'),TEMPLATE_DIR=str(source/'web/templates'),CHRONICLE_DIR=str(out/(label+'-chronicles')),REPORTS_DIR=str(out/(label+'-reports')))
  logpath=out/(label+'-server.log');log=logpath.open('w');proc=subprocess.Popen([str(binary)],cwd=source/'server',env=env,stdout=log,stderr=log)
  base=f'http://127.0.0.1:{port}'
  for _ in range(100):
   try:
    with urllib.request.urlopen(base+'/healthz',timeout=2) as r:health=json.load(r)
    break
   except Exception:
    if proc.poll() is not None:raise RuntimeError(label+' server exited; inspect PRIVATE log')
    time.sleep(.2)
  assert health['commit']==commit and health['migration']==183,health
  path=f"/api/v1/worlds/{row['world_id']}/provinces/{row['province_id']}/placements"
  request=urllib.request.Request(base+path,data=json.dumps({'target_kind':'hex','hex_ordinal':8,'good_key':'stone'}).encode(),headers={'Content-Type':'application/json','Authorization':'Bearer '+token})
  started=time.monotonic()
  try:
   with urllib.request.urlopen(request,timeout=15) as response:code=response.status;body=json.load(response)
  except urllib.error.HTTPError as e:code=e.code;body=json.load(e)
  elapsed=(time.monotonic()-started)*1000
  log.flush();errors=[line for line in logpath.read_text().splitlines() if 'eyes-tx underlying hex lock error:' in line]
  results.append({'arm':label,'health':health,'instrumentation':trace,'initial_state':row,'request':{'method':'POST','path':path,'body':{'target_kind':'hex','hex_ordinal':8,'good_key':'stone'}},'status':code,'response':body,'elapsed_ms':elapsed,'hex_lock_errors':errors})
  print(label,code,body,'hex lock errors',errors,flush=True)
 finally:
  if proc:proc.terminate();proc.wait(timeout=15)
  if log:log.close()
  for name in reversed(containers):subprocess.run(['docker','rm','-fv',name],capture_output=True)
report={'dump_sha256':hashlib.sha256(dump.read_bytes()).hexdigest(),'independent_restores':True,'production_mutated':False,'local_JWT_only':True,'arms':results,'reproduced_500_to_201':results[0]['status']==500 and results[1]['status']==201}
(out/'snapshot-repro.json').write_text(json.dumps(report,indent=2));print('reproduced_500_to_201',report['reproduced_500_to_201'],flush=True)
