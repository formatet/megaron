#!/usr/bin/env python3
"""Physical mutants, fresh DB each, exact byte restore even on failure."""
from pathlib import Path
import os,subprocess,hashlib,json
ROOT=Path(__file__).resolve().parents[1];OUT=ROOT/'docs/reviews/hex-for-hex-frakt-bud';OUT.mkdir(parents=True,exist_ok=True)
cases=[
 ('freight-early-hex','server/internal/transport/transport.go','r := j.Route(departedTick, dueTick)','r := j.Route(departedTick, dueTick)\n r.Version=0;r.OffsetMilli=0','./internal/transport','TestTimedFreightDispatchKeepsStartHexAndDeadline'),
 ('runner-early-eye','server/internal/province/eyes.go','p, err := r.Position(now)','r.Version=0;r.OffsetMilli=0\n p, err := r.Position(now)','./internal/messenger','TestTimedRunnerReturnWaitsForReachedHex'),
 ('port-early-boarding','server/internal/messenger/passage.go','h.clk.Now().Before(ready) || boarded.Before(ready)','h.clk.Now().Add(time.Duration(r.TickNanoseconds)/1000).Before(ready) || boarded.Add(time.Duration(r.TickNanoseconds)/1000).Before(ready)','./internal/messenger','TestTimedRunnerCannotBoardBeforePortOrCatchDepartedShip'),
 ('freight-window','server/internal/transport/intercept.go','if j.Clock != nil && !j.Route(0, 0).InTransitAt(now)','if false && j.Clock != nil && !j.Route(0, 0).InTransitAt(now)','./internal/transport','TestTimedFreightCatchupOnlyInterceptsActiveWindow'),
 ('freight-deadline','server/internal/events/scheduler.go',"'TransportArrivalV2','TradeDeliveryV2'","'TradeDeliveryV2'",'./internal/events','TestFreightDeadlineTypesWaitForExactInstant'),
]
results=[]
for i,(name,file,old,new,pkg,test) in enumerate(cases):
 path=ROOT/file;original=path.read_bytes();source=original.decode();assert old in source,(name,old)
 try:
  path.write_text(source.replace(old,new))
  with (OUT/(name+'-mutant.log')).open('w') as log:
   run=subprocess.run(['tools/gotest.sh',pkg,'-run','^'+test+'$','-v'],cwd=ROOT,env={**os.environ,'GOTEST_PORT':str(56301+i)},stdout=log,stderr=subprocess.STDOUT)
  output=(OUT/(name+'-mutant.log')).read_text()
  assert run.returncode!=0 and '--- FAIL: '+test in output and '[build failed]' not in output,(name,output)
 finally:path.write_bytes(original)
 assert path.read_bytes()==original
 results.append({'mutant':name,'test':test,'assertion_red':True,'restored_sha256':hashlib.sha256(original).hexdigest()})
(OUT/'mutations.json').write_text(json.dumps(results,indent=2))
print('PASS 5 assertion-red physical mutants; exact byte restores')
