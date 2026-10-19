#!/usr/bin/env python3
"""Compare production bytes to baseline and mutate each text color in place."""
from pathlib import Path
import subprocess,json,re
HERE=Path(__file__).resolve().parent;ROOT=HERE.parents[2]
def old(path):return subprocess.check_output(['git','-C',str(ROOT),'show','07e2eac4:'+str(path.relative_to(ROOT))],text=True)
css=ROOT/'web/static/megaron.css';original=css.read_bytes();text=original.decode()
reverted=re.sub(r'  /\* Text on papyrus:[\s\S]*?  /\* Notification chip tokens \*/','  /* Notification chip tokens */',text)
reverted=reverted.replace('var(--safe-text)','var(--safe)')
assert reverted==old(css),'unexpected CSS changes beyond text tokens'
changed=[]
for p in (ROOT/'web/static/js/megaron').rglob('*.js'):
 before=old(p);after=p.read_text()
 assert after.replace('var(--safe-text)','var(--safe)').replace('var(--accent-city-text)','var(--accent-city)')==before,p
 if before!=after:changed.append(str(p.relative_to(ROOT)))
results=[]
try:
 for key,good,bad in [('safe','#786200','#D4AC0D'),('trade','#7C5F1B','#C89A2B'),('city','#356095','#3E6BAA'),('oikos','#436B47','#4E7A52'),('system','#646260','#6C6A68')]:
  css.write_bytes(original.replace(good.encode(),bad.encode()))
  r=subprocess.run(['node','--test','web/static/js/megaron/palette.test.mjs'],cwd=ROOT,text=True,capture_output=True)
  assert r.returncode!=0 and ('--safe-text' if key=='safe' else '--accent-'+key+'-text') in r.stdout
  results.append(dict(mutation=key,exit_code=r.returncode,output=r.stdout))
  css.write_bytes(original);assert css.read_bytes()==original
  r=subprocess.run(['node','--test','web/static/js/megaron/palette.test.mjs'],cwd=ROOT,text=True,capture_output=True);assert r.returncode==0
  results[-1]['restore_green']=True
finally:css.write_bytes(original)
(HERE/'mutations.json').write_text(json.dumps(results,indent=2)+'\n')
(HERE/'containment.json').write_text(json.dumps(dict(baseline='07e2eac4',css_only_text_tokens=True,js_only_text_tokens=changed,mutations='5 RED → exact restore → fresh GREEN'),indent=2)+'\n')
# Exact usage references, including pending decision tokens, no hand-maintained list.
lines=['# Användningsinventering\n\nBas 07e2eac4; radnummer i slice 23. Text, ikoner och ytor blandas här; ingen automatisk omfärgning.\n']
for key in ['--danger','--accent-diplomacy','--text-dim']:
 lines.append('\n## '+key+'\n\n```text\n')
 for p in [css,*sorted((ROOT/'web/static/js').rglob('*.js')),*sorted((ROOT/'web/templates').rglob('*.html'))]:
  for no,line in enumerate(p.read_text().splitlines(),1):
   if key in line:lines.append(f'{p.relative_to(ROOT)}:{no}: {line.strip()}\n')
 lines.append('```\n')
(HERE/'uses.md').write_text(''.join(lines))
print('Containment GREEN; five mutations RED/restore GREEN; usage inventory written')
