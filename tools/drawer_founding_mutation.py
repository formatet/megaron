#!/usr/bin/env python3
"""Actual consumer/helper mutations; named red, always restore, green. Usage: OUT."""
from pathlib import Path
import os,subprocess,sys
ROOT=Path(__file__).resolve().parents[1];OUT=Path(sys.argv[1]);OUT.mkdir(parents=True,exist_ok=True)
cases=[
 ('close','ui/drawers/economy.js',"  window.closeDrawer('economy');",'', 'ui/drawers/economy_city.test.mjs','Economy must close before City opens'),
 ('dialog','render/map.js',"    if (pending) return;","    if (pending) return;\n    if (!confirm('Found?')) return;",'render/founding_inline.test.mjs','founding must confirm inline without browser dialog'),
 ('pending','render/map.js','    if (pending) return;','', 'render/founding_inline.test.mjs','pending founding cannot reopen confirmation'),
 ('double','ui/inline_result.js','    if (sent) return;','', 'render/founding_inline.test.mjs','one founding request while pending'),
]
for name,file,needle,replacement,test,message in cases:
 source=ROOT/'web/static/js/megaron'/file;original=source.read_text();assert original.count(needle)==1
 def run():return subprocess.run(['node','--test',str(ROOT/'web/static/js/megaron'/test)],cwd=ROOT,env={k:os.environ[k] for k in ('HOME','PATH')},capture_output=True,text=True)
 try:
  source.write_text(original.replace(needle,replacement));r=run();(OUT/(name+'-red.log')).write_text(r.stdout+r.stderr);assert r.returncode and message in r.stdout,(name,r.stdout)
 finally:source.write_text(original)
 r=run();(OUT/(name+'-restored.log')).write_text(r.stdout+r.stderr);assert r.returncode==0,(name,r.stdout)
 print(name+': named red -> restored green')
