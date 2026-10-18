#!/usr/bin/env python3
"""Physical compiling mutations; named RED -> byte-exact restore -> fresh GREEN."""
from pathlib import Path
import hashlib,os,socket,subprocess
ROOT=Path(__file__).resolve().parents[3];OUT=Path(__file__).resolve().parent
cases=[
 ('stale-starvation','server/internal/kharis/tick.go','AND e.world_tick=current_world_tick()','AND true',['tools/gotest.sh','./internal/kharis','-run','TestStarvationSignal_NoFalseAuditAfterPaidUpkeep','-v'],'TestStarvationSignal_NoFalseAuditAfterPaidUpkeep'),
 ('loss-location-missing','server/internal/combat/upkeep.go','payload["garrison_settlement_id"] = *u.settlementID','payload["former_garrison"] = *u.settlementID',['tools/gotest.sh','./internal/combat','-run','TestStarvationSignal_ActualLossEvenLastGarrison','-v'],'TestStarvationSignal_ActualLossEvenLastGarrison'),
 ('last-garrison-silent','server/internal/kharis/tick.go',"AND e.payload->>'garrison_settlement_id'=s.id::text","AND e.payload->>'garrison_settlement_id'=s.id::text AND EXISTS (SELECT 1 FROM units u WHERE u.settlement_id=s.id AND u.status='garrison' AND u.size>0)",['tools/gotest.sh','./internal/combat','-run','TestStarvationSignal_ActualLossEvenLastGarrison','-v'],'TestStarvationSignal_ActualLossEvenLastGarrison') ,('wrong-city-starvation','server/internal/kharis/tick.go',"AND e.payload->>'garrison_settlement_id'=s.id::text","AND true",['tools/gotest.sh','./internal/combat','-run','TestStarvationSignal_ActualLossEvenLastGarrison','-v'],'TestStarvationSignal_ActualLossEvenLastGarrison')
]
# Optional last-case-only run after adding the two-city fixture.
import sys
if len(sys.argv)>1: cases=cases[int(sys.argv[1]):]
for name,relative,old,new,cmd,named in cases:
 p=ROOT/relative;original=p.read_bytes();source=original.decode();assert source.count(old)==1
 def run(label,green):
  env={k:os.environ[k] for k in ('HOME','PATH')}
  with socket.socket() as sock:sock.bind(('127.0.0.1',0));env['GOTEST_PORT']=str(sock.getsockname()[1])
  r=subprocess.run(cmd,cwd=ROOT,env=env,capture_output=True,text=True);log=r.stdout+r.stderr;(OUT/(name+'-'+label+'.log')).write_text(log)
  if green:assert r.returncode==0,(name,log[-3000:])
  else:assert r.returncode and named in log and '--- FAIL:' in log and '[build failed]' not in log,(name,log[-3000:])
 try:p.write_text(source.replace(old,new));run('red',False)
 finally:p.write_bytes(original)
 run('restored',True);assert p.read_bytes()==original
 print(name,'named RED -> exact bytes -> fresh GREEN',hashlib.sha256(original).hexdigest(),flush=True)
