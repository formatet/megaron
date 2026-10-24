#!/usr/bin/env python3
"""Run actual K2 source mutants against independent fresh migrated databases."""
from pathlib import Path
import subprocess
import sys
from datetime import datetime
import hashlib

root = Path(__file__).resolve().parent.parent
out = root / 'docs/reviews/correspondence-k2/server-evidence'
out.mkdir(exist_ok=True)
mutants = [
 ('successor-recipient', 'server/db/migrations/207_correspondence_cases.up.sql',
  'recipient_id=person AND delivered', 'true', 'TestCorrespondenceCasePrivacySnapshotsAndDeletion'),
 ('owner-at-delivery', 'server/db/migrations/207_correspondence_cases.up.sql',
  'recipient_id=delivery_owner', 'true', 'TestCorrespondenceChangedOwnerBeforeDeliveryGetsNoPrivateLetter'),
 ('reply-before-return', 'server/api/handlers/correspondence_cases.go',
  'CASE WHEN recipient_id=$2 OR returned THEN reply_text ELSE NULL END', 'reply_text', 'TestCorrespondenceCasePrivacySnapshotsAndDeletion'),
 ('case-participant', 'server/api/handlers/correspondence_cases.go',
  'AND ($2=first_player_id OR $2=second_player_id)', 'AND true', 'TestCorrespondenceCaseRejectsUnrelatedPersonAndWrongWorld'),
]
for name, relative, old, new, test in mutants:
 path = root / relative
 original = path.read_bytes()
 stamp=datetime.now().astimezone().isoformat()
 with (out/'mutation-intervals.log').open('a') as audit: audit.write(f'{name} START {stamp} original_sha256={hashlib.sha256(original).hexdigest()}\n')
 text = original.decode()
 if text.count(old) != 1:
  raise RuntimeError(f'{name}: expected one mutation target, found {text.count(old)}')
 try:
  path.write_text(text.replace(old, new))
  proc = subprocess.run([str(root/'tools/gotest.sh'), './api/handlers', '-run', '^'+test+'$'], cwd=root, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
  log = proc.stdout.decode()
  (out / (name+'.log')).write_text(log)
  if proc.returncode == 0 or ('--- FAIL: '+test) not in log:
   raise RuntimeError(f'{name}: expected named behavioral RED, got exit {proc.returncode}\n{log}')
  print(name+': named RED', flush=True)
 finally:
  path.write_bytes(original)
  with (out/'mutation-intervals.log').open('a') as audit: audit.write(f'{name} RESTORED {datetime.now().astimezone().isoformat()} sha256={hashlib.sha256(path.read_bytes()).hexdigest()}\n')
  if path.read_bytes() != original:
   raise RuntimeError('restore failed: '+relative)
proc = subprocess.run([str(root/'tools/gotest.sh'), './api/handlers', './internal/messenger', '-run', 'Test(SendCreatesPersistentCaseIdentity|Correspondence|Reply_|Host)'], cwd=root, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
(out/'restored.log').write_bytes(proc.stdout)
sys.stdout.write(proc.stdout.decode())
sys.exit(proc.returncode)
