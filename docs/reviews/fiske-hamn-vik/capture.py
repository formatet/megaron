#!/usr/bin/env python3
"""Read-only live snapshot using the repository's transaction-enforced DB reader."""
from pathlib import Path
import sys,json
root=Path(__file__).resolve().parents[3];out=Path(__file__).resolve().parent
sys.path.insert(0,str(root/'tools'))
from alfa_db import Db
rows=Db(ssh='root@10.0.1.92').rows((out/'snapshot.sql').read_text())
assert len(rows)==1 and rows[0]['transaction_read_only']=='on'
(out/'live-readonly.json').write_text(json.dumps(rows[0],indent=2,ensure_ascii=False)+'\n')
print(json.dumps({'observed_at':rows[0]['observed_at'],'read_only':rows[0]['transaction_read_only'],'migration':rows[0]['migration'],'cities':len(rows[0]['cities'] or [])}))
