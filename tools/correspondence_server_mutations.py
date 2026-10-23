#!/usr/bin/env python3
"""Physical source mutants; fresh DB per arm, exact restore in finally."""
import hashlib,pathlib,subprocess
ROOT=pathlib.Path(__file__).resolve().parents[1]
OUT=ROOT/'docs/reviews/correspondence-k1';OUT.mkdir(parents=True,exist_ok=True)
def check(label,pattern,red=False):
 result=subprocess.run([str(ROOT/'tools/gotest.sh'),'./api/handlers','-run',pattern],cwd=ROOT,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
 (OUT/(label+'.log')).write_bytes(result.stdout)
 if red:
  assert result.returncode!=0 and ('--- FAIL: '+pattern).encode() in result.stdout, result.stdout.decode()
 else:assert result.returncode==0,result.stdout.decode()
def arm(path,label,mutate,pattern):
 original=path.read_bytes();digest=hashlib.sha256(original).hexdigest()
 try:
  altered=mutate(original);assert altered!=original
  path.write_bytes(altered);check(label,pattern,True)
 finally:path.write_bytes(original)
 assert hashlib.sha256(path.read_bytes()).hexdigest()==digest
check('server-before-mutation','TestCorrespondenceQueue')
arm(ROOT/'server/api/handlers/messenger.go','baseline-deadline',lambda _:subprocess.check_output(['git','-C',str(ROOT),'show','09237d7f:server/api/handlers/messenger.go']),'TestCorrespondenceQueueDeadlineBlocksLateAcceptAndDeclineBeforeWorker')
arm(ROOT/'server/api/handlers/messenger_queue.go','mutant-expired-action',lambda b:b.replace(b'if offer.Status == "pending" && m.ExpiresAt != nil',b'if false && offer.Status == "pending" && m.ExpiresAt != nil'),'TestCorrespondenceQueueKeepsExpiredAndInsolventWithLiveActions')
arm(ROOT/'server/api/handlers/messenger_queue.go','mutant-update-membership',lambda b:b.replace(b'm.correspondence_created_xid',b'm.xmin::text::xid8'),'TestCorrespondenceQueueKeysetSurvivesEqualTimesNewMailAndResolution')
arm(ROOT/'server/api/handlers/messenger_queue.go','mutant-restore-membership',lambda b:b.replace(b'm.correspondence_created_epoch<>$8',b'($8::timestamptz IS NULL)'), 'TestCorrespondenceQueueLogicalRestoreDoesNotHideHistoricalLetters')
check('server-restored','TestCorrespondenceQueue')
print('4 named source failures; byte-exact restored; fresh restored queue contracts GREEN')
