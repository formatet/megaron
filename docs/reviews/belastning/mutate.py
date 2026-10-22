#!/usr/bin/env python3
"""Drop population from ONE reader; real full-tick/projection test must disagree."""
from pathlib import Path
import subprocess,json,hashlib
out=Path(__file__).resolve().parent;root=out.parents[2];p=root/'server/internal/kharis/project.go';original=p.read_bytes()
needle=b'dailyDecay := computeDailyDecay(cityCount, population)';assert original.count(needle)==1
try:
 p.write_bytes(original.replace(needle,b'dailyDecay := computeDailyDecay(cityCount, 0)',1))
 with (out/'mutation-red.log').open('w') as log:
  result=subprocess.run(['bash','tools/gotest.sh','./internal/kharis','-run','^TestPopulationBurdenFullTickMatchesProjection$'],cwd=root,stdout=log,stderr=subprocess.STDOUT)
 text=(out/'mutation-red.log').read_text()
 assert result.returncode!=0 and 'population projection net' in text and '--- FAIL: TestPopulationBurdenFullTickMatchesProjection' in text,text
finally:p.write_bytes(original)
assert p.read_bytes()==original
with (out/'mutation-restored-green.log').open('w') as log:
 result=subprocess.run(['bash','tools/gotest.sh','./internal/kharis','-run','TestPopulationBurden|TestRealmDecay|TestRealmKharis|TestEarlyTempleOfferingLocalGiftAndProjection|TestTickNoticeFailureKeepsWorldRunning'],cwd=root,stdout=log,stderr=subprocess.STDOUT)
assert result.returncode==0,'restored tests failed'
(out/'mutation.json').write_text(json.dumps({'mutation':'drop residents only in ProjectDailyNet','red':True,'byte_exact_restore':True,'restored_green':True,'project_sha256':hashlib.sha256(original).hexdigest()},indent=2)+'\n')
