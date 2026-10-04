#!/usr/bin/env python3
"""Prepare an isolated local game process; never imports the developer .env."""
import argparse,json,os,secrets,subprocess
from pathlib import Path

def main():
 p=argparse.ArgumentParser(description=__doc__);p.add_argument('action',choices=['prepare','build','run']);p.add_argument('--dir',required=True,type=Path);p.add_argument('--matrix-fixture',type=Path);a=p.parse_args()
 repo=Path(__file__).resolve().parents[2];a.dir.mkdir(mode=0o700,parents=True,exist_ok=True);os.chmod(a.dir,0o700)
 config=a.dir/'game-env.json'
 if a.action=='prepare':
  if not a.matrix_fixture:raise SystemExit('--matrix-fixture is required')
  m=json.loads(a.matrix_fixture.read_text())
  if not m['base_url'].startswith('http://127.0.0.1:'):raise SystemExit('Only isolated localhost Matrix is allowed')
  env={'DATABASE_URL':'postgres://agora_acceptance:agora_acceptance_local_fixture@127.0.0.1:18432/agora_acceptance?sslmode=disable','REDIS_URL':'127.0.0.1:18379','JWT_SECRET':'agora-acceptance-'+secrets.token_urlsafe(32),'PORT':'18097','TICK_SECONDS':'6','MAP_WIDTH':'30','MAP_HEIGHT':'20','WORLD_NAME':'Agora acceptance','POLEIA_ADMIN_KEY':'agora-acceptance-'+secrets.token_urlsafe(24),'POLEIA_AGORA_URL':m['base_url'],'POLEIA_AGORA_ACCESS_TOKEN':m['temenos_token'],'POLEIA_AGORA_ADMIN_ROOM':m['room'],'STATIC_DIR':str(repo/'web/static'),'TEMPLATE_DIR':str(repo/'web/templates'),'CHRONICLE_DIR':str(a.dir/'chronicles'),'REPORTS_DIR':str(a.dir/'reports')}
  fd=os.open(config,os.O_WRONLY|os.O_CREAT|os.O_TRUNC,0o600)
  with os.fdopen(fd,'w') as f:json.dump(env,f)
  print('Private game config prepared; no game process started.');return
 # Go needs HOME/PATH for its cache and toolchain, but game settings are explicit.
 env={k:os.environ[k] for k in ['PATH','HOME','GOCACHE','GOMODCACHE','GOTOOLCHAIN'] if k in os.environ}
 if a.action=='build':
  commit=subprocess.check_output(['git','-C',str(repo),'rev-parse','--short=7','HEAD'],text=True).strip()
  if subprocess.run(['git','-C',str(repo),'diff','--quiet','HEAD','--']).returncode:commit+='+dirty'
  subprocess.run(['go','build','-ldflags','-X main.buildCommit='+commit,'-o',str(a.dir/'temenos'),'./cmd/server'],cwd=repo/'server',env=env,check=True);print('Isolated game binary built.');return
 env.update(json.loads(config.read_text()))
 # Existing server PORT behavior binds :18097 on all interfaces. Requests use
 # localhost; this is a temporary acceptance process, not a localhost-only bind.
 print('Starting temporary game process on :18097; existing all-interface bind.',flush=True)
 os.chdir(repo/'server');os.execve(str(a.dir/'temenos'),[str(a.dir/'temenos')],env)
if __name__=='__main__':main()
