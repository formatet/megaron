#!/usr/bin/env python3
"""Read-only DB snapshot. All game DB reads share one SELECT/transaction snapshot."""
import json,sys,subprocess
from pathlib import Path
p=Path(__file__).resolve().parent
root=p.parents[2]
sys.path.insert(0,str(root/'tools'))
from alfa_db import Db
health=subprocess.run(['ssh','root@10.0.1.92','curl -fsS http://127.0.0.1:8080/healthz'],capture_output=True,text=True,check=True)
(p/'healthz.json').write_text(health.stdout+'\n')
s=Db(ssh='root@10.0.1.92').rows((p/'snapshot.sql').read_text())[0]
assert s['transaction_read_only']=='on'
(p/'live-snapshot.json').write_text(json.dumps(s,indent=2,ensure_ascii=False)+'\n')
print(json.dumps({'observed_at':s['observed_at'],'migration':s['migration'],'cities':len(s['cities']),'units':len(s['units']),'events':len(s['recent_events'])}))
