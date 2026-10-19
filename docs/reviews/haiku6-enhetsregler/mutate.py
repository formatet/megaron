#!/usr/bin/env python3
"""An invented 100-man march gate must break the completed-64 HTTP case."""
from pathlib import Path
import subprocess,json
HERE=Path(__file__).resolve().parent;ROOT=HERE.parents[2]
p=ROOT/'server/internal/combat/march_start.go';saved=p.read_bytes()
old=b'if u.Status != unit.StatusGarrison && u.Status != unit.StatusPositioned {'
new=b'if (u.Status != unit.StatusGarrison && u.Status != unit.StatusPositioned) || u.Size < 100 {'
assert saved.count(old)==1
args=[str(ROOT/'tools/gotest.sh'),'./api/handlers/','-run','TestMarch_ExploreCompleted64AndForming40','-v']
try:
 p.write_bytes(saved.replace(old,new))
 r=subprocess.run(args,cwd=ROOT,text=True,capture_output=True)
 output=r.stdout+r.stderr
 assert r.returncode!=0 and 'FAIL: TestMarch_ExploreCompleted64AndForming40/completed_after_losses' in output,output
 (HERE/'mutation-red.log').write_text(output)
finally:p.write_bytes(saved)
assert p.read_bytes()==saved
r=subprocess.run(args,cwd=ROOT,text=True,capture_output=True)
(HERE/'mutation-restored.log').write_text(r.stdout+r.stderr)
assert r.returncode==0,r.stdout+r.stderr
print('Invented size gate: named RED → byte-exact restore → fresh PG GREEN')
