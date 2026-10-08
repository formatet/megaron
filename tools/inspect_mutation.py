#!/usr/bin/env python3
"""Physical inspect consumer mutations, named red/restored green. Usage: OUT."""
from pathlib import Path
import os,subprocess,sys
ROOT=Path(__file__).resolve().parents[1];OUT=Path(sys.argv[1]);OUT.mkdir(parents=True,exist_ok=True)
mapfile='web/static/js/megaron/render/map.js';test=ROOT/'web/static/js/megaron/render/inspect_simple.test.mjs'
cases=[
 ('words',mapfile,'numberWords(days)','numberWords(days / 24)','host food uses exact unscaled game days'),
 ('empty',mapfile,"defended ? 'Defended'","true ? 'Defended'",'zero defenders and no walls means no defenders seen'),
 ('stale',mapfile,"document.getElementById('ip-defence-row').style.display === 'none'",'false','late army reply must not change fog panel'),
 ('blocks','web/static/map.html','<div id="ip-body-extra"></div>','<div id="ip-culture-row">Culture</div><div id="ip-body-extra"></div>','obsolete inspect blocks removed'),
]
for name,file,needle,replacement,message in cases:
 source=ROOT/file;original=source.read_text();assert original.count(needle)==1
 def run():return subprocess.run(['node','--test',str(test)],cwd=ROOT,env={k:os.environ[k] for k in ('HOME','PATH')},capture_output=True,text=True)
 try:
  source.write_text(original.replace(needle,replacement));r=run();(OUT/(name+'-red.log')).write_text(r.stdout+r.stderr);assert r.returncode and message in r.stdout,(name,r.stdout)
 finally:source.write_text(original)
 r=run();(OUT/(name+'-restored.log')).write_text(r.stdout+r.stderr);assert r.returncode==0,(name,r.stdout)
 print(name+': named red -> restored green')
