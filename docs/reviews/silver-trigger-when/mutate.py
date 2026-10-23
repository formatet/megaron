#!/usr/bin/env python3
"""Remove WHEN physically; every arm uses a fresh migrated database."""
from pathlib import Path
import hashlib,subprocess,json
out=Path(__file__).resolve().parent;root=out.parents[2];p=root/'server/db/migrations/203_silver_trigger_when.up.sql';original=p.read_bytes()
needle=b"FOR EACH ROW WHEN (NEW.good_key = 'silver')";assert original.count(needle)==1
cmd=['tools/gotest.sh','./internal/combat','-run','TestSilverTrigger']
try:
 p.write_bytes(original.replace(needle,b'FOR EACH ROW'))
 with (out/'mutation-red.log').open('w') as log:r=subprocess.run(cmd,cwd=root,stdout=log,stderr=subprocess.STDOUT)
 assert r.returncode!=0 and 'actual trigger calls silver=1 other=16' in (out/'mutation-red.log').read_text()
finally:p.write_bytes(original)
assert p.read_bytes()==original
with (out/'mutation-restored-green.log').open('w') as log:r=subprocess.run(cmd,cwd=root,stdout=log,stderr=subprocess.STDOUT)
assert r.returncode==0
(out/'mutation.json').write_text(json.dumps({'physical_mutant':'WHEN removed from actual migration203','red':True,'restored_green':True,'up_sha256':hashlib.sha256(original).hexdigest()},indent=2)+'\n')
