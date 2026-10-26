#!/usr/bin/env python3
"""Verify final production SHA against executed pursuit mutants and emit exact sed recipes."""
from pathlib import Path
import hashlib,json,re,shlex,subprocess,tempfile
root=Path(__file__).resolve().parents[1];out=root/'docs/reviews/bud-foljer'
rows=json.loads((out/'server-mutations/verified.json').read_text())+json.loads((out/'client-mutations/verified.json').read_text())['mutants']
lines=['# Exact sed recipes','','Use a separate clean checkout; restore each file before the next recipe. Commands byte-checked against the executed manifest substitutions.',''];matches=[]
for row in rows:
 p=root/row['file'];original=p.read_bytes();actual=hashlib.sha256(original).hexdigest()
 assert actual==row.get('sha256',row.get('source_sha256')),row['file']
 matches.append({'file':row['file'],'sha256':actual})
 if not row.get('assertion_red'):continue
 old,new=row['old'],row['new'];text=original.decode();assert text.count(old)==1
 pattern=r'\n'.join(re.escape(v).replace('#',r'\#') for v in old.split('\n'))
 replacement=new.replace('\\','\\\\').replace('&',r'\&').replace('#',r'\#').replace('\n',r'\n');expression='s#'+pattern+'#'+replacement+'#'
 with tempfile.NamedTemporaryFile(mode='w+') as tmp:
  tmp.write(text);tmp.flush();subprocess.run(['sed','-z','-E','-i',expression,tmp.name],check=True)
  assert Path(tmp.name).read_text()==text.replace(old,new),row['name']
 lines+=['## '+row['name'],'','```sh','sed -z -E -i '+shlex.quote(expression)+' '+shlex.quote(row['file']),'```','','Expected assertion: `'+row['expected_test']+'`.','']
(out/'exact-sed.md').write_text('\n'.join(lines));(out/'final-source-match.json').write_text(json.dumps(matches,indent=2)+'\n')
print('PASS source SHA for',len(rows),'executed mutants/survivors; exact sed byte-checks')
