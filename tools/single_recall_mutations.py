#!/usr/bin/env python3
"""Mutate single recall arrival reuse, failure notice/audit and frozen claim."""
from pathlib import Path
import re
import subprocess
import sys
ROOT=Path(__file__).resolve().parents[1]
OUT=Path(sys.argv[1] if len(sys.argv)>1 else '/tmp/megaron-single-recall-mutations')
OUT.mkdir(parents=True,exist_ok=True)
def run(name,pattern,red=False):
    result=subprocess.run(['tools/gotest.sh','./internal/messenger','-run',pattern],cwd=ROOT,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
    (OUT/(name+'.log')).write_text(result.stdout)
    failures=re.findall(r'--- FAIL: (\S+)',result.stdout)
    if red:assert result.returncode and any(re.search(pattern,x) for x in failures) and 'build failed' not in result.stdout,(name,result.stdout)
    else:assert result.returncode==0,(name,result.stdout)
    print(name+(': RED '+','.join(failures) if red else ': GREEN'),flush=True)
run('baseline','TestSingleRecall')
mutations=[
 ('arrival-reuse','server/internal/combat/recall_redirect.go','if !arrivalQueued {','if true {','TestSingleRecall_IdenticalActiveArrival'),
 ('failure-notice','server/internal/messenger/order_delivery_failure.go','if h.hub != nil {','if false && h.hub != nil {','TestSingleRecall_FailureAfterClaimIsNamedAndAuditedOnce'),
 ('failure-audit','server/internal/messenger/order_delivery_failure.go','const OrderDeliveryFailed = "OrderDeliveryFailed"','const OrderDeliveryFailed = "WrongAudit"','TestSingleRecall_FailureAfterClaimIsNamedAndAuditedOnce'),
 ('frozen-claim','server/internal/messenger/order_delivery.go','if ct.RowsAffected() == 0 {','if false && ct.RowsAffected() == 0 {','TestSingleRecall_OldSharedMessengerOnlyTurnsFirst'),
]
for name,file,old,new,pattern in mutations:
    path=ROOT/file;original=path.read_text();assert old in original,name
    try:
        path.write_text(original.replace(old,new,1));subprocess.run(['gofmt','-w',str(path)],check=True)
        run(name,pattern,True)
    finally:path.write_text(original)
run('restored','TestSingleRecall')
