#!/usr/bin/env python3
"""Physical river, ford selection, mission and preview mutants; restore bytes."""
from pathlib import Path
import subprocess, hashlib
root=Path(__file__).resolve().parents[3]
out=Path(__file__).resolve().parent
arms=[
 ('river','web/static/js/megaron/render/markraster.js',"const isWaterTerrain = t => t === 'deep_sea' || t === 'coastal_sea' || t === 'river';", "const isWaterTerrain = t => t === 'deep_sea' || t === 'coastal_sea';", 'river march: real map destination'),
 ('ford','web/static/js/megaron/ui/marchctx.js'," || (dest.terrain === 'river_ford' && u.category === 'naval')", '', 'river march: real map destination'),
 ('mission','web/static/js/megaron/ui/marchctx.js',"if (dest.terrain !== 'river_ford' || category !== 'naval')", 'if (true)', 'ford send uses the same per-unit mission'),
 ('preview','web/static/js/megaron/ui/march_preview.js','...(pick.order || {})','...{}', 'ford preview preserves separate'),
]
for name,relative,old,new,assertion in arms:
 p=root/relative;data=p.read_bytes();text=data.decode();assert text.count(old)==1,(name,text.count(old))
 try:
  p.write_text(text.replace(old,new))
  run=subprocess.run(['node','--test','web/static/js/megaron/ui/marchctx.test.mjs','web/static/js/megaron/ui/march_preview.test.mjs'],cwd=root,capture_output=True,text=True)
  (out/(name+'-red.log')).write_text(run.stdout+run.stderr)
  assert run.returncode and 'not ok' in run.stdout and assertion in run.stdout,(name,run.stdout)
 finally:p.write_bytes(data)
 run=subprocess.run(['node','--test','web/static/js/megaron/ui/marchctx.test.mjs','web/static/js/megaron/ui/march_preview.test.mjs'],cwd=root,capture_output=True,text=True)
 (out/(name+'-restored.log')).write_text(run.stdout+run.stderr)
 assert run.returncode==0,(name,run.stdout)
 assert p.read_bytes()==data
 print(name, 'named RED -> identical bytes -> GREEN', hashlib.sha256(data).hexdigest())
