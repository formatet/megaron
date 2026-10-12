#!/usr/bin/env python3
"""Physical strict retreat mode and sealed-letter arrival mutations."""
import pathlib
import subprocess
ROOT=pathlib.Path(__file__).resolve().parents[1]
OUT=ROOT/'docs/reviews/retreat-tva-lagen'
OUT.mkdir(parents=True,exist_ok=True)
def mutation(label,path,old,new,package,test,expected):
 file=ROOT/path;original=file.read_bytes()
 try:
  source=original.decode();assert source.count(old)==1,(label,source.count(old));file.write_text(source.replace(old,new,1))
  result=subprocess.run(['tools/gotest.sh',package,'-run',test],cwd=ROOT,text=True,capture_output=True);output=result.stdout+result.stderr
  (OUT/('mutation-server-'+label+'.log')).write_text(output)
  assert result.returncode!=0 and expected in output,output
  print(label+': named red',flush=True)
 finally:file.write_bytes(original)
mutation('domain-percentage','server/internal/combat/retreat_mode.go',
 'if retreatAtLoss != nil {','if false {',
 './internal/combat/','TestRetreatTwoModesDomainRejectsPercentageEvenWithValidMode','retreat_at_loss domain guard missing')
mutation('realm-percentage','server/api/handlers/retreat_default.go',
 'if len(req.RetreatAtLoss) > 0 {','if false {',
 './api/handlers/','TestRequireStartedWorld_NoOrdersUntilThresholdJoins','realm retreat_at_loss request accepted')
mutation('before-courier','server/api/handlers/unit.go',
 'if len(req.RetreatAtLoss) > 0 {','if false {',
 './api/handlers/','TestStandingOrders_FieldUnitOrderTravelsByCourierAndAppliesOnlyToActiveBattle','retreat_at_loss was accepted before courier dispatch')
mutation('legacy-arrival','server/internal/messenger/order_delivery.go',
 'if order.RetreatAtLoss != nil {','if false {',
 './api/handlers/','TestStandingOrders_FieldUnitOrderTravelsByCourierAndAppliesOnlyToActiveBattle','legacy retreat courier did not translate to loyalty on arrival')
with (OUT/'restored-server.log').open('w') as log:
 subprocess.run(['tools/gotest.sh','./internal/combat/','./api/handlers/','./internal/messenger/','-run','TestRetreat|TestSetStandingOrders|TestStandingOrders_Field|TestRequireStartedWorld'],cwd=ROOT,stdout=log,stderr=subprocess.STDOUT,check=True)
print('server: restored green')
