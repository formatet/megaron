#!/usr/bin/env python3
"""Remove the mobile toolbar reservation physically; require named red and restore bytes."""
from pathlib import Path
import hashlib,json,subprocess,sys
ROOT=Path(__file__).resolve().parents[1];OUT=ROOT/'docs/reviews/mobilkarta/topbar-v2/mutation';OUT.mkdir(parents=True,exist_ok=True)
path=ROOT/'web/static/megaron.css';before=path.read_bytes();rule=b'  .gt-celestial { flex: 0 0 60%; min-width: 0; gap: 4px; padding: 0 4px; }\n';assert before.count(rule)==1
try:
 path.write_bytes(before.replace(rule,b''))
 run=subprocess.run([sys.executable,'tools/mobilkarta_browser.py','--topbar-only','--browsers','firefox','--output',str(OUT/'browser')],cwd=ROOT,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
 (OUT/'red.log').write_text(run.stdout)
 assert run.returncode!=0 and 'U topbar buttons are within viewport and reachable at 390' in run.stdout,run.stdout
 rows=json.loads((OUT/'browser/proof.json').read_text());failed=[r for r in rows if r['mode']=='mobile' and 'buttons' in r];assert failed and any(not b['inside'] for b in failed[-1]['buttons']),rows
finally:
 path.write_bytes(before);assert path.read_bytes()==before
(OUT/'proof.json').write_text(json.dumps(dict(named_failure='U topbar buttons are within viewport and reachable at 390',exit_code=run.returncode,restored_byte_identically=True,css_sha256=hashlib.sha256(before).hexdigest()),indent=2)+'\n')
print('Mobile topbar reservation: named RED → byte-identical restore',flush=True)
