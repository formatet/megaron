#!/usr/bin/env python3
"""Read-only live definition via shared Db wrapper. No credentials dumped."""
import sys,json
from pathlib import Path
sys.dont_write_bytecode=True
out=Path(__file__).resolve().parent;root=out.parents[2];sys.path.insert(0,str(root/'tools'))
from alfa_db import Db
db=Db(ssh='root@10.0.1.92')
rows=db.rows("""SELECT now() AS observed_at,current_setting('transaction_read_only') AS transaction_read_only,
 (SELECT version FROM schema_migrations) AS migration,
 pg_get_triggerdef(t.oid) AS trigger_definition,pg_get_expr(t.tgqual,t.tgrelid) AS when_expression,
 pg_get_functiondef(t.tgfoid) AS function_definition
 FROM pg_trigger t WHERE t.tgrelid='settlement_goods'::regclass AND t.tgname='silver_mining_credit'""")
assert len(rows)==1 and rows[0]['transaction_read_only']=='on'
(out/'live-inventory.json').write_text(json.dumps(rows,indent=2)+'\n')
print(json.dumps({k:v for k,v in rows[0].items() if k!='function_definition'},indent=2))
