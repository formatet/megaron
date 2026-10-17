#!/usr/bin/env python3
"""Physically restore the old olive renderer: visible-change proof must go red."""
from pathlib import Path
import hashlib,subprocess
ROOT=Path(__file__).resolve().parents[3];OUT=Path(__file__).resolve().parent
path=ROOT/'web/static/js/megaron/render/map.js';original=path.read_bytes()
old=subprocess.check_output(['git','-C',str(ROOT),'show','2bf95ac7:web/static/js/megaron/render/map.js'])
try:
 path.write_bytes(old)
 result=subprocess.run(['python3',str(OUT/'proof.py')],cwd=ROOT,capture_output=True,text=True)
 (OUT/'mutation-red.log').write_text(result.stdout+result.stderr)
 assert result.returncode and 'no visible change' in result.stderr,(result.returncode,result.stderr[-1500:])
finally:path.write_bytes(original)
result=subprocess.run(['python3',str(OUT/'proof.py')],cwd=ROOT,capture_output=True,text=True)
(OUT/'mutation-restored.log').write_text(result.stdout+result.stderr)
assert result.returncode==0,result.stderr
assert path.read_bytes()==original
print('old renderer -> named no-visible-change RED -> identical bytes -> all-scene GREEN',hashlib.sha256(original).hexdigest())
