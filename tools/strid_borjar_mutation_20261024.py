#!/usr/bin/env python3
"""Fresh-DB deduplication mutation; always restore source bytes before exiting."""
from pathlib import Path
import subprocess,hashlib
root=Path(__file__).resolve().parents[1];p=root/'server/internal/combat/battle_begun.go';before=p.read_bytes()
needle=" AND NOT EXISTS (SELECT 1 FROM notifications n WHERE n.world_id=$2 AND n.player_id=d.owner_id AND n.kind='BattleBegun' AND n.body_json->>'battle_id'=$1::text)"
assert needle in before.decode()
try:
 p.write_text(before.decode().replace(needle,''))
 with (root/'docs/reviews/strid-borjar/mutation-red.log').open('w') as log:
  result=subprocess.run([str(root/'tools/gotest.sh'),'./internal/combat/','-run','^TestBattleBegunMultipleOwnersCityAndNewDefender$'],cwd=root,stdout=log,stderr=subprocess.STDOUT)
 assert result.returncode!=0,'dedupe mutant survived'
 assert 'recipients after reinforcements=' in (root/'docs/reviews/strid-borjar/mutation-red.log').read_text(),'failed for wrong reason'
finally:
 p.write_bytes(before)
assert hashlib.sha256(p.read_bytes()).digest()==hashlib.sha256(before).digest()
with (root/'docs/reviews/strid-borjar/mutation-restored-green.log').open('w') as log:
 subprocess.run([str(root/'tools/gotest.sh'),'./internal/combat/','-run','^TestBattleBegun'],cwd=root,stdout=log,stderr=subprocess.STDOUT,check=True)
print('PASS: removed per-battle/per-Wanax archive dedupe -> RED; exact restore -> GREEN')
