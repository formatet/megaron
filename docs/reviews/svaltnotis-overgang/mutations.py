#!/usr/bin/env python3
"""Physical named mutations with byte-exact restoration and fresh PG after each."""
from pathlib import Path
import hashlib,json,os,socket,subprocess
HERE=Path(__file__).resolve().parent;ROOT=HERE.parents[2]
food='server/internal/economy/food_tick.go';owner='server/internal/kharis/transition_notices.go';web='web/static/js/megaron/ui/notification_state.js'
audit='_, _ = h.store.Append(ctx, settlementID, events.StreamProvince, EventFoodShortfall, payload, worldID, nil)'
daily=audit+'''
        if h.hub != nil {
            var ownerID uuid.UUID
            _ = h.pool.QueryRow(ctx, `SELECT owner_id FROM settlements WHERE id=$1`, settlementID).Scan(&ownerID)
            _ = h.hub.NotifyPlayer(ctx,worldID,ownerID,EventFoodShortfall,2,map[string]any{"settlement_id":settlementID,"name":settlementName,"unmet":unmet})
        }'''
cases=[
 ('daily-inbox-restored',food,audit,daily,'./internal/economy/','TestFoodTick_ShortfallKeepsNamedAuditWithoutDailyNotification'),
 ('civilian-onset-suppressed',owner,'if c.unmet > 0 {','if false {','./internal/kharis/','TestCivilianHungerSeventyTicksAuditAndTransitions'),
 ('daily-transition-restored',owner,'if prior == next {','if false {','./internal/kharis/','TestCivilianHungerSeventyTicksAuditAndTransitions'),
 ('shared-client-hunger-key',web,"const key = (n.kind.startsWith('Civilian') ? 'civilian:' : 'garrison:') + scope;",'const key = scope;',None,'civilian hunger and garrison hunger clear independently in the same city'),
]
cases.append(('civilian-inertia-removed',owner,'const civilianRecoveryTicks = 5','const civilianRecoveryTicks = 1','./internal/kharis/','TestCivilianHungerAlternatingMealsStayOnePeriod'))
results=[]
for name,path,before,after,package,test in cases:
 p=ROOT/path;original=p.read_bytes();assert original.count(before.encode())==1,(name,'ambiguous mutation')
 def run(phase):
  if package:
   with socket.socket() as listener:listener.bind(('127.0.0.1',0));port=listener.getsockname()[1]
   command=[str(ROOT/'tools/gotest.sh'),package,'-run','^'+test+'$'];env=dict(os.environ,GOTEST_PORT=str(port))
  else:command=['node','--experimental-default-type=module','--test','web/static/js/megaron/ui/notification_state.test.mjs'];env={k:os.environ[k] for k in ['HOME','PATH']}
  result=subprocess.run(command,cwd=ROOT,env=env,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True)
  (HERE/(name+'-'+phase+'.log')).write_text(result.stdout)
  if package:assert 'färsk DB på migration 189' in result.stdout,(name,'no fresh189')
  return result
 try:
  p.write_bytes(original.replace(before.encode(),after.encode()))
  red=run('red');marker=('--- FAIL: '+test) if package else ('not ok 4 - '+test)
  assert red.returncode!=0 and marker in red.stdout,(name,'not named RED',red.stdout)
 finally:p.write_bytes(original)
 assert p.read_bytes()==original
 green=run('restored-green');assert green.returncode==0,(name,'restore failure',green.stdout)
 results.append(dict(mutation=name,test=test,source=path,sha256=hashlib.sha256(original).hexdigest(),result='named RED -> byte-exact restore -> fresh GREEN'))
 print(results[-1],flush=True)
(HERE/'mutations.json').write_text(json.dumps(results,indent=2)+'\n')
