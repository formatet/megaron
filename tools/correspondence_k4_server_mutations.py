#!/usr/bin/env python3
"""K4 behavioral source mutations, each against a fresh independently migrated DB."""
from pathlib import Path
from datetime import datetime
import hashlib
import subprocess
import sys

root=Path(__file__).resolve().parent.parent
out=root/'docs/reviews/correspondence-k4/server-evidence'
out.mkdir(exist_ok=True)
path=root/'server/api/handlers/correspondence_memory.go'
mutants=[
 ('remote-acceptance-before-return', 'l.sender_id=$2 AND l.returned AND (l.returned_epoch<>$5 OR pg_visible_in_snapshot(l.returned_xid,$4::pg_snapshot))','l.sender_id=$2','TestCorrespondenceMemoryAcceptedTermsRequireVisibleAcceptance'),
 ('other-person-private-citation','WHERE n.player_id=$1 AND c.world_id=$2','WHERE true AND c.world_id=$2','TestCorrespondenceMemoryCitationsAreExactPrivateAndHistoricallyAttributed'),
 ('inexact-invented-quote','if !strings.Contains(text, req.Quote) {','if !strings.Contains(text, req.Quote) && false {','TestCorrespondenceMemoryRejectsUnauthorizedOrInexactCitation'),
 ('hidden-reply-citation','(l.recipient_id=$3 OR l.returned)','true','TestCorrespondenceMemoryCitationsAreExactPrivateAndHistoricallyAttributed'),
 ('later-return-enters-snapshot','AND (l.returned_epoch<>$5 OR pg_visible_in_snapshot(l.returned_xid,$4::pg_snapshot))','','TestCorrespondenceMemoryTransactionPaginationFreezesAcceptanceAndReturn'),
 ('later-acceptance-enters-snapshot','AND (l.accepted_epoch<>$5 OR pg_visible_in_snapshot(l.accepted_xid,$4::pg_snapshot))','','TestCorrespondenceMemoryRecipientPaginationFreezesAcceptance'),
]
for name,old,new,test in mutants:
 original=path.read_bytes();mutated=original.decode()
 if mutated.count(old)!=1:raise RuntimeError(f'{name}: expected one target, got {mutated.count(old)}')
 with (out/'mutation-intervals.log').open('a') as audit:audit.write(f'{name} START {datetime.now().astimezone().isoformat()} original_sha256={hashlib.sha256(original).hexdigest()}\n')
 try:
  path.write_text(mutated.replace(old,new))
  proc=subprocess.run([str(root/'tools/gotest.sh'),'./api/handlers','-run','^'+test+'$'],cwd=root,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
  log=proc.stdout.decode();(out/(name+'.log')).write_text(log)
  if proc.returncode==0 or '--- FAIL: '+test not in log:raise RuntimeError(f'{name}: expected named RED, exit{proc.returncode}\n{log}')
  print(name+': named RED',flush=True)
 finally:
  path.write_bytes(original)
  with (out/'mutation-intervals.log').open('a') as audit:audit.write(f'{name} RESTORED {datetime.now().astimezone().isoformat()} sha256={hashlib.sha256(path.read_bytes()).hexdigest()}\n')
  if path.read_bytes()!=original:raise RuntimeError('restore mismatch')
proc=subprocess.run([str(root/'tools/gotest.sh'),'./api/handlers','-run','^TestCorrespondence(Memory|Private|Archive|Search|Case|Changed|Separate|Sender)'],cwd=root,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
(out/'restored.log').write_bytes(proc.stdout);sys.stdout.write(proc.stdout.decode());sys.exit(proc.returncode)
