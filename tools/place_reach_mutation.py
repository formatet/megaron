#!/usr/bin/env python3
"""Physical first-owner and authoritative movement-hook mutations."""
import pathlib,subprocess
ROOT=pathlib.Path(__file__).resolve().parents[1];OUT=ROOT/'docs/reviews/flod-doparratt';f=ROOT/'server/internal/combat/place_reach.go';original=f.read_bytes()
def mutation(label,old,new,expected,test='TestPlaceReachMovementHookFirstOwnerAndRerun'):
 try:
  s=original.decode();assert old in s;f.write_text(s.replace(old,new,1))
  r=subprocess.run(['tools/gotest.sh','./internal/combat/','-run',test],cwd=ROOT,text=True,capture_output=True);text=r.stdout+r.stderr
  (OUT/f'mutation-{label}.log').write_text(text)
  assert r.returncode and expected in text,text
  print(label+': named red')
 finally:f.write_bytes(original)
mutation('first-owner','AND claimed_by IS NULL RETURNING named_tick','RETURNING named_tick','earliest physical entry did not retain first owner')
mutation('movement-hook','outcomes, err := recordPlaceEntriesTx(ctx, tx, h.store, e.WorldID, int64(e.DueTick)*1000)','var outcomes []expeditionOutcome; err = nil','physical movement reach hook missing')
mutation('arrival-prerequisite','if err := h.flushEntries(ctx, e); err != nil {\n\t\t\treturn err\n\t\t}', '', 'catch-up arrival erased last reached hex', 'TestPlaceReachWorkerCatchupArrivalFlushesLastHexSameBatch')
with (OUT/'restored-reach.log').open('w') as log:subprocess.run(['tools/gotest.sh','./internal/combat/','-run','TestPlaceReach'],cwd=ROOT,stdout=log,stderr=subprocess.STDOUT,check=True)
print('reach: restored green')
