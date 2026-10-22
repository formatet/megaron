#!/usr/bin/env python3
import sys,json,subprocess
from pathlib import Path
out=Path(__file__).resolve().parent;sys.path.insert(0,str(out.parents[2]/'tools'))
from alfa_db import Db
health=subprocess.run(['ssh','root@10.0.1.92','curl -fsS http://127.0.0.1:8080/healthz'],text=True,capture_output=True,check=True)
(out/'healthz.json').write_text(health.stdout+'\n')
s=Db(ssh='root@10.0.1.92').rows((out/'snapshot.sql').read_text())[0]
assert s['transaction_read_only']=='on'
(out/'live-snapshot.json').write_text(json.dumps(s,ensure_ascii=False,indent=2)+'\n')
print(s['observed_at'],s['tick'],len(s['wanaxes']),len(s['cities']))
