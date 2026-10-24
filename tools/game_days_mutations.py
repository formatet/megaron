#!/usr/bin/env python3
"""Restore bare days on one player string per surface; require the guard RED."""
from pathlib import Path
import hashlib,json,subprocess
ROOT=Path(__file__).resolve().parents[1]
OUT=ROOT/'docs/reviews/game-days/mutations';OUT.mkdir(parents=True,exist_ok=True)
cases=[('web','web/static/js/megaron/ui/march_preview.js','silver per game day in the field','silver per day in the field'),('keryx','server/cmd/keryx/output.go','%+.1f per game day','%+.1f per day'),('codex','web/static/codex/time.md','food for 12 game days','food for 12 days'),('server','server/api/handlers/opening_dispatch.go','Time. A game day passes','Time. A day passes')]
results=[]
for surface,rel,old,new in cases:
 p=ROOT/rel;original=p.read_bytes();digest=hashlib.sha256(original).hexdigest()
 try:
  text=original.decode();assert text.count(old)==1;p.write_text(text.replace(old,new))
  r=subprocess.run(['python3',str(ROOT/'tools/day_guard.py'),surface],cwd=ROOT,capture_output=True,text=True)
  (OUT/(surface+'-red.log')).write_text(r.stdout+r.stderr)
  assert r.returncode!=0 and rel in r.stdout,(surface,r.stdout,r.stderr)
 finally:p.write_bytes(original)
 assert hashlib.sha256(p.read_bytes()).hexdigest()==digest
 green=subprocess.run(['python3',str(ROOT/'tools/day_guard.py'),surface],capture_output=True,text=True)
 (OUT/(surface+'-restored-green.log')).write_text(green.stdout+green.stderr);assert green.returncode==0
 results.append({'surface':surface,'mutation':new,'red':True,'restored_sha256':digest,'restored_green':True})
 print(surface+': RED -> exact restore -> GREEN',flush=True)
(OUT/'results.json').write_text(json.dumps(results,indent=2)+'\n')
