#!/usr/bin/env python3
"""Physical regressions: lost population, soft zoom, closed horizon sample."""
from pathlib import Path
import hashlib,importlib.util,json,subprocess,sys
HERE=Path(__file__).resolve().parent;source=HERE/'generate.py';original=source.read_bytes()
spec=importlib.util.spec_from_file_location('verify21c',HERE/'verify.py');verify=importlib.util.module_from_spec(spec);spec.loader.exec_module(verify)
mutations=[
 ('fixed-population',b's=fn(pop);structure,life=s.overlay_life()',b's=fn(4500);structure,life=s.overlay_life()'),
 ('soft-zoom',b'Image.Resampling.NEAREST',b'Image.Resampling.BILINEAR'),
 ('closed-mountain-sample',b's.finishing();return s',b"s.finishing();s.rect((65,0,95,20),'earth') if s.tier==2 else None;return s"),
]
results=[]
try:
 for name,old,new in mutations:
  assert old in original
  try:
   source.write_bytes(original.replace(old,new,1));subprocess.run([sys.executable,str(source)],check=True,capture_output=True)
   try:verify.check()
   except AssertionError as e:results.append(dict(mutation=name,result='RED',reason=str(e)))
   else:raise AssertionError('mutation survived: '+name)
  finally:
   source.write_bytes(original);assert source.read_bytes()==original
   subprocess.run([sys.executable,str(source)],check=True,capture_output=True);verify.check()
   if results:results[-1]['restore']='byte-exact source; fresh generated checks GREEN'
finally:
 source.write_bytes(original);subprocess.run([sys.executable,str(source)],check=True,capture_output=True)
(HERE/'mutation-results.json').write_text(json.dumps(dict(source_sha256=hashlib.sha256(original).hexdigest(),results=results),indent=2)+'\n');print(json.dumps(results))
