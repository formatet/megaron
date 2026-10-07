#!/usr/bin/env python3
"""Physical J mutations: More visibility, authoritative coverage and reserve rules.
Usage: python3 tools/production_mutations.py OUT
"""
from pathlib import Path
import os, subprocess, sys
ROOT=Path(__file__).resolve().parents[1]
OUT=Path(sys.argv[1]);OUT.mkdir(parents=True,exist_ok=True)
source=ROOT/'web/static/js/megaron/ui/drawers/city_production.js'
test=source.with_name('city_production.test.mjs')
arms=[('more','<details id="city-more" class="dsec">','<details open id="city-more" class="dsec">','More starts closed'),
 ('coverage','Food lasts ${n(s.coverage_ticks)}','Food lasts ${n(s.granary_total)}','authoritative food days'),
 ('threshold','Stores above ${n(s.high_ticks)}','Stores above ${n(s.low_ticks)}','same thresholds retained')]
for label,needle,mutation,assertion in arms:
 original=source.read_text();assert original.count(needle)==1,label
 try:
  source.write_text(original.replace(needle,mutation))
  result=subprocess.run(['node','--test',str(test)],cwd=ROOT,env={k:os.environ[k] for k in ('HOME','PATH')},capture_output=True,text=True)
  (OUT/(label+'-red.log')).write_text(result.stdout+result.stderr)
  assert result.returncode and assertion in result.stdout,(label,result.stdout)
 finally:source.write_text(original)
 result=subprocess.run(['node','--test',str(test)],cwd=ROOT,env={k:os.environ[k] for k in ('HOME','PATH')},capture_output=True,text=True)
 (OUT/(label+'-restored.log')).write_text(result.stdout+result.stderr)
 assert result.returncode==0,label
 print(label+': named assertion red -> restored green',flush=True)
