#!/usr/bin/env python3
"""Physically limit each client loop to its first unit; require named assertion red.
No DB needed: these client tests use callbacks / authenticated httptest HTTP.
Sources always restored; then both actual client test suites must pass.
Usage: python3 tools/recall_all_mutations.py OUT
"""
import os
from pathlib import Path
import subprocess
import sys
ROOT=Path(__file__).resolve().parents[1]
OUT=Path(sys.argv[1]).resolve();OUT.mkdir(parents=True,exist_ok=True)
ENV={k:os.environ[k] for k in ('HOME','PATH')}
js=ROOT/'web/static/js/megaron/ui/recall_all.js'
go=ROOT/'server/cmd/keryx/cmd_recall_all.go'
arms=[(js,"units.filter(u => u.status === 'marching')","units.filter(u => u.status === 'marching').slice(0, 1)",['node','--test','web/static/js/megaron/ui/recall_all.test.mjs'],ROOT,'every marching unit must receive its own recall attempt','web'),
      (go,'results = append(results, result)','results = append(results, result)\n\t\tbreak',['go','test','./cmd/keryx/','-run','TestRecallAll','-count=1'],ROOT/'server','must recall every marching unit despite rejection','cli')]
for path,needle,mutation,cmd,cwd,assertion,label in arms:
 original=path.read_text();assert original.count(needle)==1
 try:
  path.write_text(original.replace(needle,mutation))
  with (OUT/(label+'-red.log')).open('w') as log:
   result=subprocess.run(cmd,cwd=cwd,env=ENV,stdout=log,stderr=subprocess.STDOUT)
  text=(OUT/(label+'-red.log')).read_text()
  assert result.returncode and assertion in text,(label,text)
 finally:path.write_text(original)
 with (OUT/(label+'-restored.log')).open('w') as log:
  result=subprocess.run(cmd,cwd=cwd,env=ENV,stdout=log,stderr=subprocess.STDOUT)
 assert result.returncode==0,label
 print(label+': named assertion red -> restored green',flush=True)
