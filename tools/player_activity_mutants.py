#!/usr/bin/env python3
"""Physical auth activity mutants, each on a fresh DB, in a separate clean checkout.
Usage: python3 tools/player_activity_mutants.py CHECKOUT OUTPUT
Requires assertion-red (not build failure), exact restoration, then fresh green.
"""
from pathlib import Path
import hashlib,json,subprocess,sys,os,socket
checkout=Path(sys.argv[1]).resolve();out=Path(sys.argv[2]).resolve();out.mkdir(parents=True,exist_ok=True)
assert not subprocess.check_output(['git','-C',str(checkout),'status','--porcelain'],text=True).strip(),'use a clean separate checkout'
source=subprocess.check_output(['git','-C',str(checkout),'rev-parse','HEAD'],text=True).strip()
call='svc.recordActivity(r.Context(), claims.PlayerID)'
cases=[(f'middleware-{mode}','server/internal/auth/middleware.go',call,'// activity observation removed',i,'TestAuthenticatedActivityLifecycle') for i,mode in enumerate(['api','web','optional'])]
cases += [
 ('db-gate','server/internal/auth/activity.go',"(last_seen_at IS NULL OR last_seen_at <= $2::timestamptz - interval '5 minutes')",'true',0,'TestActivityConcurrentRequestsWriteOnce'),
 ('cache-gate','server/internal/auth/activity.go','ok && now.Before(next)','ok && false && now.Before(next)',0,'TestAuthenticatedActivityLifecycle'),
 ('failure-retry','server/internal/auth/activity.go','delete(s.activityNext, playerID)','// failed observation cache kept',0,'TestActivityFailureDoesNotRejectAuthentication'),
 ('five-minute-boundary','server/internal/auth/activity.go',"interval '5 minutes'","interval '4 minutes'",0,'TestActivityFiveMinuteGateAndCacheExpiry'),
 ('null-observation','server/internal/auth/activity.go','last_seen_at IS NULL OR ','',0,'TestAuthenticatedActivityLifecycle'),
 ('migration-no-backfill','server/db/migrations/216_player_activity.up.sql','TIMESTAMPTZ;','TIMESTAMPTZ DEFAULT now();',0,'TestAuthenticatedActivityLifecycle'),
]
def test_env():
 with socket.socket() as sock:
  sock.bind(('127.0.0.1',0));port=sock.getsockname()[1]
 return {**os.environ,'GOTEST_PORT':str(port)}
manifest=[]
for name,rel,old,new,nth,expected in cases:
 p=checkout/rel;original=p.read_bytes();s=original.decode();positions=[];start=0
 while True:
  pos=s.find(old,start)
  if pos<0:break
  positions.append(pos);start=pos+len(old)
 assert len(positions)>nth,(name,positions)
 pos=positions[nth];mutated=s[:pos]+new+s[pos+len(old):]
 try:
  p.write_text(mutated)
  result=subprocess.run([str(checkout/'tools/gotest.sh'),'./internal/auth','-run','TestAuthenticatedActivity|TestActivity'],cwd=checkout,env=test_env(),text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
  (out/(name+'.log')).write_text(result.stdout)
  assert result.returncode and '--- FAIL: '+expected in result.stdout and '[build failed]' not in result.stdout,(name,result.stdout[-2000:])
  manifest.append({'name':name,'file':rel,'old':old,'new':new,'occurrence':nth,'expected_test':expected,'assertion_red':True,'source_commit':source,'source_sha256':hashlib.sha256(original).hexdigest()})
  print('assertion-red',name,flush=True)
 finally:
  p.write_bytes(original);assert p.read_bytes()==original
result=subprocess.run([str(checkout/'tools/gotest.sh'),'./internal/auth'],cwd=checkout,env=test_env(),text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
(out/'restored-green.log').write_text(result.stdout);assert result.returncode==0,result.stdout
(out/'verified.json').write_text(json.dumps(manifest,indent=2)+'\n')
assert not subprocess.check_output(['git','-C',str(checkout),'status','--porcelain'],text=True).strip()
print('PASS',len(cases),'assertion-red mutants, exact restore, fresh auth green')
