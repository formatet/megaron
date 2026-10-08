#!/usr/bin/env python3
"""Physical inspect consumer mutations, named red/restored green. Usage: OUT."""
from pathlib import Path
import os,subprocess,sys
ROOT=Path(__file__).resolve().parents[1];OUT=Path(sys.argv[1]);OUT.mkdir(parents=True,exist_ok=True)
mapfile='web/static/js/megaron/render/map.js';test=ROOT/'web/static/js/megaron/render/host_simple.test.mjs'
cases=[
 ('net',mapfile,"net >= 0 ? 'yes' : 'no'","net >= 0 ? 'no' : 'yes'",'negative net must not promise self-sufficiency'),
 ('details',mapfile,'<details id="ip-host-details" class="dsec">','<details id="ip-host-details" class="dsec" open>','Details must begin closed'),
 ('stone',mapfile,"potential('stone')","potential('lumber')",'zero stone gives Stone no'),
 ('stale',mapfile,'if (!stillThisHost()) return;','if (false) return;','late Host forecast must not update another panel'),
]
for name,file,needle,replacement,message in cases:
 source=ROOT/file;original=source.read_text();assert original.count(needle)==1
 def run():return subprocess.run(['node','--test',str(test)],cwd=ROOT,env={k:os.environ[k] for k in ('HOME','PATH')},capture_output=True,text=True)
 try:
  source.write_text(original.replace(needle,replacement));r=run();(OUT/(name+'-red.log')).write_text(r.stdout+r.stderr);assert r.returncode and message in r.stdout,(name,r.stdout)
 finally:source.write_text(original)
 r=run();(OUT/(name+'-restored.log')).write_text(r.stdout+r.stderr);assert r.returncode==0,(name,r.stdout)
 print(name+': named red -> restored green')
