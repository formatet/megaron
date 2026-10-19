#!/usr/bin/env python3
"""Physical source mutations, fresh node process per arm, byte-exact restore."""
from pathlib import Path
import hashlib,json,subprocess
HERE=Path(__file__).resolve().parent;ROOT=HERE.parents[2]
source=ROOT/'web/static/js/megaron/render/city.js';original=source.read_bytes();text=original.decode()
test=['node','--experimental-default-type=module','--test','web/static/js/megaron/render/city.test.mjs']
mutations=[
 ('constant-population', 'const terraceLevels = [1, 3, 5][tier];','const terraceLevels = 1;','terraced housing grows geometrically'),
 ('missing-wall','const walls = Math.max(0, Math.min(3, Number(sett.walls) || 0));','const walls = 0;','wall silhouette and gate'),
 ('missing-hunger',"sett.food?.growth_state === 'shrinking'",'false','food truth comes from growth state'),
 ('missing-siege','if (besieged) {','if (false) {','all statuses stay in separate layers'),
 ('missing-construction','for (const item of [...buildQueue].sort','for (const item of [].sort','all statuses stay in separate layers'),
 ('dark-horizon',"sky: '#E3C6A0'","sky: '#896858'",'terraced housing grows geometrically'),
]
results=[]
try:
 for name,before,after,expected in mutations:
  assert text.count(before)==1,(name,text.count(before))
  source.write_text(text.replace(before,after))
  run=subprocess.run(test,cwd=ROOT,capture_output=True,text=True);output=run.stdout+run.stderr
  (HERE/(name+'-red.log')).write_text(output)
  assert run.returncode!=0 and any('not ok' in line and expected in line for line in output.splitlines()),(name,output)
  source.write_bytes(original)
  green=subprocess.run(test,cwd=ROOT,capture_output=True,text=True)
  assert green.returncode==0,green.stdout+green.stderr
  assert source.read_bytes()==original
  results.append(dict(mutation=name,expected_failure=expected,result='named RED -> byte-exact restore -> fresh GREEN'))
finally:source.write_bytes(original)
(HERE/'mutations.json').write_text(json.dumps(dict(source_sha256=hashlib.sha256(original).hexdigest(),mutations=results),indent=2)+'\n')
print(json.dumps(results))
