#!/usr/bin/env python3
"""Capture payroll facts via the shared transaction-enforced read-only reader."""
from pathlib import Path
import json,sys
root=Path(__file__).resolve().parents[3]
sys.path.insert(0,str(root/'tools'))
from alfa_db import Db
out=Path(__file__).resolve().parent
rows=Db(ssh='root@10.0.1.92').rows((out/'snapshot.sql').read_text())
assert len(rows)==1 and rows[0]['transaction_read_only']=='on'
(out/'snapshot.json').write_text(json.dumps(rows[0],indent=2,ensure_ascii=False)+'\n')
print(json.dumps({k:rows[0][k] for k in ['observed_at','transaction_read_only','migration']}))
