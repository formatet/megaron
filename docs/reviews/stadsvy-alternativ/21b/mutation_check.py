#!/usr/bin/env python3
"""Physical mutations of the parameterized study; source restored after every arm."""
from pathlib import Path
import hashlib,importlib.util,json,subprocess,sys
HERE=Path(__file__).resolve().parent;source=HERE/'generate.py';original=source.read_bytes()
spec=importlib.util.spec_from_file_location('followup_verify',HERE/'verify.py');verify=importlib.util.module_from_spec(spec);spec.loader.exec_module(verify)
mutations=[
 ('nearest-neighbour',b'Image.Resampling.NEAREST',b'Image.Resampling.BILINEAR'),
 ('population',b'self.tier=0 if population<3000 else 1 if population<8000 else 2',b'self.tier=1'),
 ('wall-level',b'self.walls=walls',b'self.walls=1'),
 ('temple-construction',b"if state=='construction':",b"if False and state=='construction':"),
]
results=[]
try:
 for name,old,new in mutations:
  assert old in original
  try:
   source.write_bytes(original.replace(old,new,1))
   subprocess.run([sys.executable,str(source)],check=True,capture_output=True)
   try:verify.check()
   except AssertionError as e:results.append(dict(mutation=name,result='RED',reason=str(e)))
   else:raise AssertionError('mutation survived: '+name)
  finally:
   source.write_bytes(original);assert source.read_bytes()==original
   subprocess.run([sys.executable,str(source)],check=True,capture_output=True)
   verify.check()
   if results:results[-1]['restore']='byte-exact source; fresh generated checks GREEN'
finally:
 source.write_bytes(original)
 subprocess.run([sys.executable,str(source)],check=True,capture_output=True)
(HERE/'mutation-results.json').write_text(json.dumps(dict(source_sha256=hashlib.sha256(original).hexdigest(),results=results),indent=2)+'\n')
print(json.dumps(results))
