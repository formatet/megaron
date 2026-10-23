#!/usr/bin/env python3
from pathlib import Path
import subprocess,hashlib,json
out=Path(__file__).resolve().parent;root=out.parents[2];p=root/'web/static/js/megaron/ui/drawers/war.js';original=p.read_bytes();needle=b'const atSea = isNaval && (isMarching || isPositioned || isFreighting);';assert original.count(needle)==1
cmd=['node','--test','web/static/js/megaron/ui/war_cards.test.mjs']
try:
 p.write_bytes(original.replace(needle,needle.replace(b' || isFreighting',b'')))
 with (out/'mutation-red.log').open('w') as log:r=subprocess.run(cmd,cwd=root,stdout=log,stderr=subprocess.STDOUT)
 assert r.returncode!=0 and 'not ok' in (out/'mutation-red.log').read_text()
finally:p.write_bytes(original)
assert p.read_bytes()==original
with (out/'mutation-restored-green.log').open('w') as log:r=subprocess.run(cmd,cwd=root,stdout=log,stderr=subprocess.STDOUT)
assert r.returncode==0
(out/'mutation.json').write_text(json.dumps({'physical_mutant':'freighting hidden again','restored_sha256':hashlib.sha256(original).hexdigest(),'red':True,'restored_green':True},indent=2)+'\n')
