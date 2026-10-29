#!/usr/bin/env python3
"""Run one player's Keryx command with a persistent, private raw audit log."""
import datetime,json,os,pathlib,subprocess,sys
player=sys.argv[1] if len(sys.argv)>1 else ''
if player not in ('gpt-damon','gpt-kleio'): raise SystemExit('unknown player')
args=sys.argv[2:]
if not args or args[0] in ('login','password','god','reports','watch','completion'): raise SystemExit('not a playtest command')
root=pathlib.Path('/home/tk/megaron-arkiv/femspelare40-20261020')
round_number=int(os.environ.get('MEGARON_PLAYTEST_ROUND','1'))
if not 1 <= round_number <= 10000: raise SystemExit('invalid round')
log=root/(player+('' if round_number==1 else f'-runda{round_number}')+'.jsonl')
count=len(log.read_text().splitlines()) if log.exists() else 0
if count>=40: raise SystemExit('40-command limit reached')
env=dict(os.environ,POLEIA_CONFIG=f'/home/tk/.config/megaron-ai/{player}.json')
start=datetime.datetime.now().astimezone().isoformat()
r=subprocess.run(['/home/tk/go/bin/poleia',*args],env=env,capture_output=True,text=True,timeout=30)
with log.open('a') as f:f.write(json.dumps(dict(n=count+1,start=start,args=args,code=r.returncode,stdout=r.stdout,stderr=r.stderr),ensure_ascii=False)+'\n')
os.chmod(log,0o600)
print(r.stdout,end='');print(r.stderr,end='',file=sys.stderr)
raise SystemExit(r.returncode)
