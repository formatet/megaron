#!/usr/bin/env python3
"""N physical consumer mutations, always restore source; named red then green."""
from pathlib import Path
import subprocess
ROOT=Path(__file__).resolve().parents[1]
p=ROOT/'web/static/js/megaron/ui/drawers/economy.js';original=p.read_text()
cases=[
 ('outbound','readStandingGoodsRows(\'out\')',"readStandingGoodsRows('home')",'N: actual POST preserves'),
 ('zero',"readStandingGoodsRows('home').map", "readStandingGoodsRows('home').filter(p => p.amount !== 0).map",'N: actual POST preserves'),
 ('crew',"const crewedBy = crew === 'to' ? to : from;",'const crewedBy = from;','N: actual POST preserves'),
 ('return',"floor: p.amount",'floor: p.amount + 1','N: actual POST preserves'),
]
try:
 for name,before,after,named in cases:
  assert before in original,name
  p.write_text(original.replace(before,after,1))
  result=subprocess.run(['node','--test','--test-name-pattern',named,str(ROOT/'web/static/js/megaron/ui/drawers/economy_rows.test.mjs')],cwd=ROOT,text=True,capture_output=True)
  assert result.returncode!=0 and 'not ok' in result.stdout,(name,result.stdout,result.stderr)
  print(name+' named red',flush=True)
  p.write_text(original)
  restored=subprocess.run(['node','--test',str(ROOT/'web/static/js/megaron/ui/drawers/economy_rows.test.mjs')],cwd=ROOT,text=True,capture_output=True)
  assert restored.returncode==0,restored.stdout+restored.stderr
  print(name+' restored green',flush=True)
finally:p.write_text(original)
