#!/usr/bin/env python3
"""Physically break quantity, War pin and landing cargo body; assert red, restore.
Usage: python3 tools/simple_march_mutations.py OUT
"""
import os
from pathlib import Path
import subprocess
import sys
ROOT=Path(__file__).resolve().parents[1]
OUT=Path(sys.argv[1]).resolve();OUT.mkdir(parents=True,exist_ok=True)
path=ROOT/'web/static/js/megaron/ui/marchctx.js'
original=path.read_text()
arms=[('quantity','return index === 0 ? group.ids.length : 0;', 'return 0;', 'all units of the first group must be ready for a single click'),
 ('pin','(!unitID || u.id === unitID)', 'true', 'War must never march another unit'),
 ('landing',"options.cargo_intent = 'colonize'", "options.cargo_intent = 'march'", 'H: landing preserves cargo-colonize/name')]
for label,needle,mutation,assertion in arms:
 assert original.count(needle)==1
 try:
  path.write_text(original.replace(needle,mutation))
  result=subprocess.run(['node','--test','web/static/js/megaron/ui/marchctx.test.mjs'],cwd=ROOT,env={k:os.environ[k] for k in ('HOME','PATH')},capture_output=True,text=True)
  (OUT/(label+'-red.log')).write_text(result.stdout+result.stderr)
  assert result.returncode and assertion in result.stdout,(label,result.stdout)
 finally:path.write_text(original)
 result=subprocess.run(['node','--test','web/static/js/megaron/ui/marchctx.test.mjs'],cwd=ROOT,capture_output=True,text=True)
 (OUT/(label+'-restored.log')).write_text(result.stdout+result.stderr)
 assert result.returncode==0,label
 print(label+': named assertion red -> restored green',flush=True)
