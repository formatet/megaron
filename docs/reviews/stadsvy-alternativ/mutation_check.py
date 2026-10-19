#!/usr/bin/env python3
"""Physical mutations to the offline image generator; always restore exact bytes."""
from pathlib import Path
import hashlib
import importlib.util
import json
import subprocess
import sys

HERE=Path(__file__).resolve().parent
SOURCE=HERE/'generate.py'
spec=importlib.util.spec_from_file_location('stadsvy_verify',HERE/'verify.py')
verify=importlib.util.module_from_spec(spec);spec.loader.exec_module(verify)
original=SOURCE.read_bytes();results=[]
mutations=[
    ('nearest-neighbour',b'Image.Resampling.NEAREST',b'Image.Resampling.BILINEAR'),
    ('construction-status',b"if state=='construction':",b"if False and state=='construction':"),
]
try:
    for name,old,new in mutations:
        assert old in original
        SOURCE.write_bytes(original.replace(old,new,1))
        subprocess.run([sys.executable,str(SOURCE)],check=True,capture_output=True)
        try:verify.check()
        except AssertionError as exc:results.append(dict(mutation=name,result='RED',reason=str(exc) or 'raster invariant rejected changed output'))
        else:raise AssertionError('mutation survived: '+name)
        finally:
            SOURCE.write_bytes(original)
            assert SOURCE.read_bytes()==original
            subprocess.run([sys.executable,str(SOURCE)],check=True,capture_output=True)
            verify.check()
            results[-1]['restored']='byte-exact, fresh artifact checks GREEN'
finally:
    SOURCE.write_bytes(original)
    subprocess.run([sys.executable,str(SOURCE)],check=True,capture_output=True)
(HERE/'mutation-results.json').write_text(json.dumps(dict(source_sha256=hashlib.sha256(original).hexdigest(),results=results),indent=2)+'\n')
print(json.dumps(results))
