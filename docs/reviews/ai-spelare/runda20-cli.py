#!/usr/bin/env python3
"""Record a single real player CLI call, with a hard per-player 40-call budget."""
import fcntl, json, os, subprocess, sys
from pathlib import Path
player=sys.argv[1]
assert player in ('gpt-idomeneus','gpt-arete')
args=sys.argv[2:]
assert args and args[0] not in ('login','register','config','auth','join'), 'existing account/world only'
log=Path(__file__).resolve().parent/(player+'-runda20-tools.jsonl')
lock=log.with_suffix('.lock').open('a')
fcntl.flock(lock,fcntl.LOCK_EX)
count=len(log.read_text().splitlines()) if log.exists() else 0
assert count<40, '40-command budget exhausted'
config=Path('/home/tk/.config/megaron-ai')/(player+'.json')
assert config.exists(),'account config missing; do not create an account'
env={k:os.environ[k] for k in ('PATH','HOME','LANG','TERM') if k in os.environ}
env['POLEIA_CONFIG']=str(config)
start=subprocess.check_output(['date','--iso-8601=seconds'],text=True).strip()
try:
 p=subprocess.run(['/home/tk/go/bin/poleia',*args],env=env,capture_output=True,text=True,timeout=45)
 record=dict(number=count+1,start=start,args=args,exit_code=p.returncode,stdout=p.stdout,stderr=p.stderr)
except subprocess.TimeoutExpired as e:
 record=dict(number=count+1,start=start,args=args,exit_code=124,stdout=str(e.stdout or ''),stderr='CLI timed out; outcome unknown, do not repeat a write')
with log.open('a') as f:f.write(json.dumps(record,ensure_ascii=False)+'\n')
print(record['stdout'],end='');print(record['stderr'],file=sys.stderr,end='')
print('\n[recorded CLI call '+str(count+1)+'/40; exit '+str(record['exit_code'])+']')
