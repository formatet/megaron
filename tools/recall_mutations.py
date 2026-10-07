#!/usr/bin/env python3
"""Fresh-DB assertion mutations for batch recall; always restore source."""
from pathlib import Path
import re
import subprocess
import sys
ROOT=Path(__file__).resolve().parents[1]
OUT=Path(sys.argv[1] if len(sys.argv)>1 else '/tmp/megaron-recall-mutations')
OUT.mkdir(parents=True,exist_ok=True)
def run(name,pattern,red=False):
    result=subprocess.run(['tools/gotest.sh','./api/handlers','-run',pattern],cwd=ROOT,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
    (OUT/(name+'.log')).write_text(result.stdout)
    failures=re.findall(r'--- FAIL: (\S+)',result.stdout)
    if red:
        assert result.returncode and any(re.search(pattern,x) for x in failures) and 'build failed' not in result.stdout,(name,result.stdout)
    else:assert result.returncode==0,(name,result.stdout)
    print(name+(': RED '+','.join(failures) if red else ': GREEN'),flush=True)
run('baseline','TestRecallBatch')
mutations=[
 ('arrival-dedup','server/internal/combat/recall_redirect.go','if !arrivalQueued {','if true {','TestRecallBatch_ReusesIdenticalPendingArrival'),
 ('worker-walltime','server/internal/combat/recall_redirect.go','return int64(delivery.DueTick) * 1000','return anchor.MilliAt(now)','TestRecallBatch_LateWorkerUsesDeliveryTickForPositionRouteAndETA'),
 ('first-member-only','server/internal/messenger/recall_group.go','for _, m := range p.RecallGroup.Members {','for _, m := range p.RecallGroup.Members[:1] {','TestRecallBatch_AllMembersReachGarrisonAndReplayIsIdempotent'),
 ('pending-guard','server/api/handlers/unit_recall_batch.go','return pending, err','return false, err','TestRecallBatch_GroupHasEveryMemberAndPartialFailures'),
 ('non-atomic-receipt','server/internal/combat/recall_redirect.go','claim, err := tx.Exec(ctx,','claim, err := pool.Exec(ctx,','TestRecallBatch_ReceiptRollsBackWithProjection'),
]
for name,file,old,new,pattern in mutations:
    path=ROOT/file;original=path.read_text();assert old in original,name
    try:
        path.write_text(original.replace(old,new,1))
        subprocess.run(['gofmt','-w',str(path)],check=True)
        run(name,pattern,True)
    finally:path.write_text(original)
run('restored','TestRecallBatch')
