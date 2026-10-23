#!/usr/bin/env python3
"""Short physical return-post source arms; fresh DB each time; exact restore."""
import hashlib,pathlib,subprocess
ROOT=pathlib.Path(__file__).resolve().parents[1];OUT=ROOT/'docs/reviews/return-post';OUT.mkdir(parents=True,exist_ok=True)
PATTERN='TestReturnPostArrivalAndNextTickUseSameBearer'
def check(label,red=False):
 r=subprocess.run([str(ROOT/'tools/gotest.sh'),'./api/handlers','-run',PATTERN],cwd=ROOT,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
 (OUT/(label+'.log')).write_bytes(r.stdout)
 assert (r.returncode!=0 and ('--- FAIL: '+PATTERN).encode() in r.stdout) if red else r.returncode==0,r.stdout.decode()
def arm(path,label,old,new):
 b=path.read_bytes();digest=hashlib.sha256(b).hexdigest();assert old in b
 try:path.write_bytes(b.replace(old,new));check(label,True)
 finally:path.write_bytes(b)
 assert hashlib.sha256(path.read_bytes()).hexdigest()==digest
check('before-mutation')
arm(ROOT/'server/internal/messenger/handler.go','mutant-long-stay',b'ReplyStayTicks   = 2 ',b'ReplyStayTicks   = 48 ')
arm(ROOT/'server/api/handlers/messenger.go','mutant-late-reply',b'if replyDeadline != nil && replyCurrentTick >= *replyDeadline',b'if false && replyDeadline != nil && replyCurrentTick >= *replyDeadline')
arm(ROOT/'server/api/handlers/messenger_queue.go','mutant-closed-reply-action',b'if m.Status == "delivered" && !m.ReplyWindowClosed',b'if m.Status == "delivered"')
check('restored')
print('3 named physical return-post failures; byte-exact restore; fresh contracts GREEN')
