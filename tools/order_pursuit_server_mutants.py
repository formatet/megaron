#!/usr/bin/env python3
"""Physical following-order mutants on a separate clean checkout, fresh DB per arm."""
from pathlib import Path
import hashlib,json,subprocess,sys,os,socket
root=Path(sys.argv[1]).resolve();out=Path(sys.argv[2]).resolve();out.mkdir(parents=True,exist_ok=True)
assert not subprocess.check_output(['git','-C',str(root),'status','--porcelain'],text=True).strip()
source=subprocess.check_output(['git','-C',str(root),'rev-parse','HEAD'],text=True).strip()
cases=[
 ('remote-meeting','server/internal/messenger/order_pursuit.go','if pos == at {','if true {','TestOrderPursuit_MovedUnitNeedsPhysicalLeg'),
 ('early-deadline','server/internal/messenger/order_pursuit.go','if now.Before(deadline) {','if false && now.Before(deadline) {','TestOrderPursuit_GuardsAndWaiting/early'),
 ('stale-generation','server/internal/messenger/order_pursuit.go','AND passage_generation=$4','AND passage_generation>=$4','TestOrderPursuit_GuardsAndWaiting/generation'),
 ('pending-witness','server/internal/messenger/order_pursuit.go','AND NOT EXISTS(SELECT 1 FROM events e','AND NOT EXISTS(SELECT 1 FROM events e',''),
 ('physical-origin','server/internal/messenger/order_pursuit.go','at, landTarget, now, arrival','province.MapPosition{Q:0,R:0}, landTarget, now, arrival','TestOrderPursuit_MovedUnitNeedsPhysicalLeg'),
 ('temporary-wait','server/internal/messenger/order_pursuit.go','p.FollowWaiting = true','p.FollowWaiting = false','TestOrderPursuit_GuardsAndWaiting/embarked'),
 ('owner-guard','server/internal/messenger/order_pursuit.go','u.OwnerID != p.PlayerID || u.WorldID != p.WorldID','false','TestOrderPursuit_GuardsAndWaiting/owner'),
 ('moving-dispatch','server/internal/messenger/order_delivery.go','if p.FollowUnit {\n\t\t\tres, err = combat.SetStanceInPursuit','if false && p.FollowUnit {\n\t\t\tres, err = combat.SetStanceInPursuit','TestOrderPursuit_MovingMeeting/stance'),
 ('moving-final-guard','server/internal/combat/march_start.go','if o.ExpectedPosition != nil && !redirecting {','if o.ExpectedPosition != nil {','TestOrderPursuit_MovingMeeting/march'),
 ('frozen-clock','server/internal/messenger/recall.go','if route.Version == 2 && !route.DepartureAt.IsZero() && route.TickNanoseconds > 0 {','if false && route.Version == 2 && !route.DepartureAt.IsZero() && route.TickNanoseconds > 0 {','TestInterceptCourierRoute_FrozenPhysicalClock'),
]
# Mutate the witness predicate itself, retaining SQL shape and all parameters.
cases[3]=('pending-witness','server/internal/messenger/order_pursuit.go','e.id>messengers.carrier_witness_id','e.id<0 AND e.id>messengers.carrier_witness_id','TestOrderPursuit_GuardsAndWaiting/witness')
manifest=[]
def run():
 with socket.socket() as s:s.bind(('127.0.0.1',0));port=s.getsockname()[1]
 return subprocess.run([str(root/'tools/gotest.sh'),'./internal/messenger','-run','TestOrderPursuit|TestInterceptCourierRoute_Frozen'],cwd=root,env={**os.environ,'GOTEST_PORT':str(port)},text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
for name,rel,old,new,test in cases:
 p=root/rel;original=p.read_bytes();s=original.decode();assert s.count(old)==1,(name,s.count(old))
 try:
  p.write_text(s.replace(old,new));r=run();(out/(name+'.log')).write_text(r.stdout)
  # Go emits nested subtest failures with spaces; the full slash path is stable.
  assert r.returncode and ('--- FAIL: '+test) in r.stdout and '[build failed]' not in r.stdout,(name,r.stdout[-3000:])
  manifest.append(dict(name=name,file=rel,old=old,new=new,expected_test=test,source_commit=source,sha256=hashlib.sha256(original).hexdigest(),assertion_red=True));print('assertion-red',name,flush=True)
 finally:p.write_bytes(original);assert p.read_bytes()==original
r=run();(out/'restored-green.log').write_text(r.stdout);assert r.returncode==0,r.stdout
(out/'verified.json').write_text(json.dumps(manifest,indent=2)+'\n')
assert not subprocess.check_output(['git','-C',str(root),'status','--porcelain'],text=True).strip()
print('PASS',len(cases),'assertion-red, exact restore, fresh green')
