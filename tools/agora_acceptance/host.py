#!/usr/bin/env python3
"""Control only this isolated Agora rig from the host process namespace."""
import argparse,json,os,signal,subprocess,time,urllib.parse
from pathlib import Path
ROOT=Path('/tmp/agora-game-private')
REPO=Path(__file__).resolve().parents[2]
EXE=ROOT/'temenos'

def require(ok,message):
 if not ok:raise RuntimeError(message)

def config():
 path=ROOT/'game-env.json'
 require(path.stat().st_uid==os.getuid() and path.stat().st_mode&0o077==0,'private config ownership/mode')
 value=json.loads(path.read_text())
 db=urllib.parse.urlparse(value['DATABASE_URL'])
 require(db.hostname=='127.0.0.1' and db.port==18432 and db.path=='/agora_acceptance','isolated database required')
 require(value['REDIS_URL']=='127.0.0.1:18379' and value['PORT']=='18097','isolated game/Redis ports required')
 require(value['POLEIA_AGORA_URL']=='http://127.0.0.1:18100','isolated Matrix required')
 require(value['STATIC_DIR']==str(REPO/'web/static') and value['TEMPLATE_DIR']==str(REPO/'web/templates'),'own worktree required')
 return value

def owned(pid):
 try:
  proc=Path('/proc')/str(pid)
  if proc.stat().st_uid!=os.getuid():return False
  target=os.readlink(proc/'exe').removesuffix(' (deleted)')
  if target!=str(EXE) or (proc/'cwd').resolve()!=REPO/'server':return False
  if (proc/'cmdline').read_bytes().split(b'\0')[0]!=str(EXE).encode():return False
  values=dict(item.split(b'=',1) for item in (proc/'environ').read_bytes().split(b'\0') if b'=' in item)
  db=urllib.parse.urlparse(values.get(b'DATABASE_URL',b'').decode())
  return db.hostname=='127.0.0.1' and db.port==18432 and values.get(b'POLEIA_AGORA_URL')==b'http://127.0.0.1:18100' and values.get(b'PORT')==b'18097'
 except (OSError,ValueError):return False

def main():
 p=argparse.ArgumentParser(description=__doc__)
 p.add_argument('action',choices=['status','serve','scenario'])
 p.add_argument('--scenario',choices=['basic','collision','outage','crash'],default='basic')
 a=p.parse_args();cfg=config()
 env={'HOME':'/home/tk','PATH':os.environ.get('PATH','/usr/bin:/bin'),'GOCACHE':'/tmp/megaron-review-gocache'}
 if a.action=='scenario':
  args=['python3',str(REPO/'tools/agora_acceptance/player.py'),'--dir','/tmp/agora-player-proof-final','--matrix-fixture','/tmp/agora-rig-clean/secret.json','--scenario',a.scenario,'--game-log',str(ROOT/'game.log')]
  if a.scenario=='basic':args+=['--keryx',str(ROOT/'keryx')]
  if a.scenario=='crash':args+=['--second-pass']
  result=subprocess.run(args,env=env)
  raise SystemExit(result.returncode)
 pids=[int(entry.name) for entry in Path('/proc').iterdir() if entry.name.isdigit() and owned(int(entry.name))]
 require(len(pids)<=1,'unexpected multiple own game processes')
 if a.action=='status':
  print('Verified isolated game processes:',pids);return
 for pid in pids:
  require(owned(pid),'process identity changed')
  os.kill(pid,signal.SIGTERM)
  for _ in range(300):
   if not owned(pid):break
   time.sleep(.1)
  require(not owned(pid),'own game did not stop')
 subprocess.run(['python3',str(REPO/'tools/agora_acceptance/game.py'),'build','--dir',str(ROOT)],env=env,check=True)
 subprocess.run(['go','build','-o',str(ROOT/'keryx'),'./cmd/keryx'],cwd=REPO/'server',env=env,check=True)
 fd=os.open(ROOT/'game.pid',os.O_WRONLY|os.O_CREAT|os.O_TRUNC,0o600);os.write(fd,str(os.getpid()).encode());os.close(fd)
 print('Starting only the verified isolated game on :18097.',flush=True)
 # Keep the foreground exec session alive. The game process receives no ambient
 # developer environment, and the log is private even when it is scanned later.
 fd=os.open(ROOT/'game.log',os.O_WRONLY|os.O_CREAT|os.O_APPEND,0o600)
 os.dup2(fd,1);os.dup2(fd,2);os.close(fd)
 env.update(cfg);os.chdir(REPO/'server');os.execve(str(EXE),[str(EXE)],env)

if __name__=='__main__':
 try:main()
 except Exception as e:raise SystemExit('Isolated host rig failed: '+type(e).__name__+' (private details withheld)') from None
