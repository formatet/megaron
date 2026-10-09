#!/usr/bin/env python3
"""Physical production mutations. Every edit is restored byte-for-byte, even on failure."""
from pathlib import Path
import hashlib,json,subprocess,sys
ROOT=Path(__file__).resolve().parents[1];OUT=ROOT/'docs/reviews/mobilkarta/mutations';OUT.mkdir(parents=True,exist_ok=True)
js=ROOT/'web/static/js/megaron/render/map_input.js';css=ROOT/'web/static/megaron.css'
cases=[
 ('touch-action',css,'#hex-canvas { touch-action: none; }','', [sys.executable,'tools/mobilkarta_browser.py','--browsers','chromium','--touch-mutation','--output',str(OUT/'touch-action-browser')],'U native touch drag keeps page still and moves camera'),
 ('drag-click',js,'if (moved(p, e)) { consumed = true; stopTimer(); }','if (moved(p, e)) { stopTimer(); }',['node','--test','web/static/js/megaron/render/map_input.test.mjs'],'not ok 6 - U motion cancels long press'),
 ('long-click',js,'        consumed = true;\n        orders(e.clientX, e.clientY);','        orders(e.clientX, e.clientY);',['node','--test','web/static/js/megaron/render/map_input.test.mjs'],'not ok 5 - U long touch/pen opens orders'),
 ('pinch-factor',js,'zoom(next.distance / pair.distance, pair.x, pair.y,','zoom(1, pair.x, pair.y,',['node','--test','web/static/js/megaron/render/map_input.test.mjs'],'not ok 7 - U pinch uses moving midpoint'),
]
results=[]
for name,path,old,new,command,expected in cases:
 before=path.read_bytes();assert before.count(old.encode())==1,(name,'mutation target')
 try:
  path.write_bytes(before.replace(old.encode(),new.encode()));run=subprocess.run(command,cwd=ROOT,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT);(OUT/(name+'.log')).write_text(run.stdout)
  assert run.returncode!=0 and expected in run.stdout,(name,run.returncode,run.stdout)
  if name=='touch-action':
   proof=json.loads((OUT/'touch-action-browser/proof.json').read_text());assert proof[0]['native']['scrollY']>50,proof
  results.append(dict(name=name,named_failure=expected,exit_code=run.returncode,before_sha256=hashlib.sha256(before).hexdigest()))
 finally:
  path.write_bytes(before);assert path.read_bytes()==before
 print(name+': named RED, byte-identical restore',flush=True)
(OUT/'proof.json').write_text(json.dumps(results,indent=2)+'\n')
run=subprocess.run(['node','--test',*sorted(str(p.relative_to(ROOT)) for p in (ROOT/'web/static/js').rglob('*.test.mjs'))],cwd=ROOT,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT);(OUT/'restored-js.log').write_text(run.stdout);assert run.returncode==0,run.stdout
print('Restored full JS: GREEN',flush=True)
