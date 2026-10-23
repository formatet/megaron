#!/usr/bin/env python3
from pathlib import Path
import hashlib,json,re
out=Path(__file__).resolve().parent;root=out.parents[2]
for line in (out/'SHA256SUMS').read_text().splitlines():
 digest,name=line.split('  ',1);assert hashlib.sha256((out/name).read_bytes()).hexdigest()==digest,name
live=json.loads((out/'live-inventory.json').read_text())[0];assert live['transaction_read_only']=='on' and live['migration']==202 and live['when_expression'] is None
assert 'actual trigger calls silver=1 other=16' in (out/'baseline-red.log').read_text()
assert 'actual trigger calls silver=1 other=16' in (out/'mutation-red.log').read_text()
assert 'FAIL' not in (out/'targeted-green.log').read_text()
assert 'FAIL' not in (out/'mutation-restored-green.log').read_text()
m=json.loads((out/'mutation.json').read_text());assert m['red'] and m['restored_green']
assert hashlib.sha256((root/'server/db/migrations/203_silver_trigger_when.up.sql').read_bytes()).hexdigest()==m['up_sha256']
full=(out/'full-fresh.log').read_text();assert 'FAIL' not in full and len(re.findall(r'^ok\s',full,re.M))==26
assert 'migration 203' in full and 'internal/world' in full
assert '# fail 0' in (out/'js-final.log').read_text();assert not (out/'vet.log').read_text()
print('Read-only live202, actual callback RED→GREEN, physical migration mutation/exact restore, full fresh203 Go/JS/vet and hashes verified.')
