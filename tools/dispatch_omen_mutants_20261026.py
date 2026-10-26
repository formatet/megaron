#!/usr/bin/env python3
"""Reproduce the recorded mutations in a separate clean checkout; restore bytes."""
import argparse,json,os,socket,subprocess
from pathlib import Path
root=Path(__file__).resolve().parents[1]
p=argparse.ArgumentParser();p.add_argument('checkout',type=Path);args=p.parse_args();checkout=args.checkout.resolve()
assert checkout!=root,'Use a separate worktree, not the evidence/source worktree'
assert not subprocess.check_output(['git','-C',str(checkout),'status','--porcelain']).strip(),'Checkout must be clean'
out=root/'docs/reviews/dispatchtext-omen/mutations'
for c in json.loads((out/'verified.json').read_text()):
 target=checkout/c['file'];original=target.read_bytes();assert c['old'] in original.decode()
 try:
  target.write_text(original.decode().replace(c['old'],c['new']))
  if c['file'].startswith('web/'):
   command=['node','--test','web/static/js/megaron/'+c['test']]
  else:
   pkg='./'+c['file'].removeprefix('server/').rsplit('/',1)[0]+'/'
   if c['name']=='shipyard-name':pkg='./api/handlers/'
   command=[str(checkout/'tools/gotest.sh'),pkg,'-run',c['test'],'-count=1']
  with socket.socket() as s:s.bind(('127.0.0.1',0));port=s.getsockname()[1]
  result=subprocess.run(command,cwd=checkout,capture_output=True,text=True,env={**os.environ,'GOTEST_PORT':str(port)})
  log=result.stdout+result.stderr
  assert result.returncode and ('--- FAIL:' in log or 'not ok' in log) and '[build failed]' not in log,(c['name'],log)
  print(c['name'],'assertion red',flush=True)
 finally:target.write_bytes(original)
print('PASS all recorded mutations; exact byte restore')
