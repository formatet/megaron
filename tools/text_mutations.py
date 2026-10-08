#!/usr/bin/env python3
"""Physical K mutations. Usage: python3 tools/text_mutations.py OUT"""
from pathlib import Path
import os, subprocess, sys
ROOT=Path(__file__).resolve().parents[1];OUT=Path(sys.argv[1]);OUT.mkdir(parents=True,exist_ok=True)
base=ROOT/'web/static/js/megaron/ui'
arms=[('group',base/'drawers/notif.js','${numberWords(count)} ${label} — show reports','+${count} ${kind} — show reports','drawers/notif_simple.test.mjs','group must use player words'),
 ('tempo',base/'drawers/notif.js','${numberWords(cal.year)}${waiting}','${numberWords(cal.year)} World speed 10×${waiting}','drawers/notif_simple.test.mjs','world speed must be absent'),
 ('abandon',base/'inline_result.js','  if (!host) return;','  if (!host) return;\n  send();','inline_result.test.mjs','opening must not dispatch'),
 ('rite',base/'drawers/kult.js',"    await loadKultDrawer();\n    showInlineResult('kult-body', d.message || (d.success ? 'The gods answered!' : 'The gods are silent.'));", "    showInlineResult('kult-body', d.message || (d.success ? 'The gods answered!' : 'The gods are silent.'));\n    await loadKultDrawer();",'drawers/inline_handlers.test.mjs','success must survive refresh')]
for label,path,needle,mutation,test,assertion in arms:
 original=path.read_text();assert original.count(needle)==1,label
 try:
  path.write_text(original.replace(needle,mutation))
  result=subprocess.run(['node','--test',str(base/test)],cwd=ROOT,env={k:os.environ[k] for k in ('HOME','PATH')},capture_output=True,text=True)
  (OUT/(label+'-red.log')).write_text(result.stdout+result.stderr)
  assert result.returncode and assertion in result.stdout,(label,result.stdout)
 finally:path.write_text(original)
 result=subprocess.run(['node','--test',str(base/test)],cwd=ROOT,env={k:os.environ[k] for k in ('HOME','PATH')},capture_output=True,text=True)
 (OUT/(label+'-restored.log')).write_text(result.stdout+result.stderr);assert result.returncode==0,label
 print(label+': named assertion red -> restored green',flush=True)
