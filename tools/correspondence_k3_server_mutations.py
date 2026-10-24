#!/usr/bin/env python3
"""Run K3 private-data source mutants, each with an independent fresh database."""
from pathlib import Path
from datetime import datetime
import hashlib
import subprocess
import sys

root = Path(__file__).resolve().parent.parent
out = root/'docs/reviews/correspondence-k3/server-evidence'
out.mkdir(exist_ok=True)
path = root/'server/api/handlers/correspondence_private.go'
mutants = [
 ('other-person-private-row', [('p.player_id=$3', 'true')], 'TestCorrespondencePrivatePersistsOnlyForOriginalPerson'),
 ('stale-overwrites-current', [('p.version=$5', 'true'), ('WHERE correspondence_private.version=$5', 'WHERE true')], 'TestCorrespondencePrivateStaleSaveAndConcurrentFirstSave'),
 ('nonparticipant-private-access', [('AND ($3=c.first_player_id OR $3=c.second_player_id)', 'AND true')], 'TestCorrespondencePrivateRejectsStrangerWrongWorldAndInvisibleCase'),
 ('undelivered-private-access', [('AND EXISTS(SELECT 1 FROM correspondence_letters l WHERE l.case_id=c.id AND (l.sender_id=$3 OR l.delivered))', '')], 'TestCorrespondencePrivatePersistsOnlyForOriginalPerson'),
]
for name,replacements,test in mutants:
 original=path.read_bytes()
 mutated=original.decode()
 for old,new in replacements:
  if mutated.count(old)!=1:
   raise RuntimeError(f'{name}: expected one target for {old!r}, got {mutated.count(old)}')
  mutated=mutated.replace(old,new)
 with (out/'mutation-intervals.log').open('a') as audit:
  audit.write(f'{name} START {datetime.now().astimezone().isoformat()} original_sha256={hashlib.sha256(original).hexdigest()}\n')
 try:
  path.write_text(mutated)
  proc=subprocess.run([str(root/'tools/gotest.sh'),'./api/handlers','-run','^'+test+'$'],cwd=root,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
  log=proc.stdout.decode()
  (out/(name+'.log')).write_text(log)
  if proc.returncode==0 or '--- FAIL: '+test not in log:
   raise RuntimeError(f'{name}: no named behavioral RED, exit{proc.returncode}\n{log}')
  print(name+': named RED',flush=True)
 finally:
  path.write_bytes(original)
  with (out/'mutation-intervals.log').open('a') as audit:
   audit.write(f'{name} RESTORED {datetime.now().astimezone().isoformat()} sha256={hashlib.sha256(path.read_bytes()).hexdigest()}\n')
  if path.read_bytes()!=original:
   raise RuntimeError('source restore mismatch')
proc=subprocess.run([str(root/'tools/gotest.sh'),'./api/handlers','-run','^TestCorrespondencePrivate'],cwd=root,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
(out/'restored.log').write_bytes(proc.stdout)
sys.stdout.write(proc.stdout.decode())
sys.exit(proc.returncode)
