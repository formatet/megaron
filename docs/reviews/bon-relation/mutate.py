#!/usr/bin/env python3
"""Physical dice regression; fresh DB per arm, byte-exact restore even on failure."""
from pathlib import Path
import subprocess,hashlib
root=Path(__file__).resolve().parents[3];out=Path(__file__).resolve().parent
path=root/'server/api/handlers/settlement.go';original=path.read_bytes()
needle=b'if !everyday {\n\t\tvar roll int'
assert original.count(needle)==1
try:
 path.write_bytes(original.replace(needle,b'if true {\n\t\tvar roll int',1))
 with (out/'mutation-red.log').open('w') as log:
  result=subprocess.run(['bash','tools/gotest.sh','./api/handlers','-run','^TestRelationEverydayExactThresholdNoDiceOrStandingDebit$'],cwd=root,stdout=log,stderr=subprocess.STDOUT)
 text=(out/'mutation-red.log').read_text()
 assert result.returncode!=0 and 'exact threshold response 200' in text and '--- FAIL: TestRelationEverydayExactThreshold' in text,text
finally:path.write_bytes(original)
assert path.read_bytes()==original
with (out/'mutation-restored-green.log').open('w') as log:
 result=subprocess.run(['bash','tools/gotest.sh','./api/handlers','./cmd/keryx','-run','TestRelation|TestPrayerPayment|TestAlpha|TestRiteOffering|TestRiteList'],cwd=root,stdout=log,stderr=subprocess.STDOUT)
assert result.returncode==0,'restored tests failed'
(out/'mutation.json').write_text(__import__('json').dumps({'mutation':'everyday prayers consult legacy dice','red':True,'byte_exact_restore':True,'source_sha256':hashlib.sha256(original).hexdigest(),'fresh_restored_green':True},indent=2)+'\n')
