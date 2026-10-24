#!/usr/bin/env python3
"""K2 client mutants: stable IDs, private-archive route, stale world response.
Run in the owned K2 worktree; restores bytes after each mutant.
"""
from pathlib import Path
import hashlib,json,subprocess
ROOT=Path(__file__).resolve().parents[1]
OUT=ROOT/'docs/reviews/correspondence-k2/client'
OUT.mkdir(parents=True,exist_ok=True)
results=[]
js=['node','--test','web/static/js/megaron/ui/drawers/diplomacy_archive.test.mjs']
go=['go','test','./cmd/keryx','-run','TestPersonalCorrespondence','-count=1']
for name,path,old,new,cmd,cwd,expected in [
 ('case-key-is-name','web/static/js/megaron/ui/drawers/diplomacy_archive.js','data-case-open="${esc(c.id)}"','data-case-open="${esc(c.counterpart_name)}"',js,ROOT,'personal cases retain stable IDs'),
 ('late-world-response','web/static/js/megaron/ui/drawers/diplomacy_archive.js','State.WORLD_ID === world &&','true &&',js,ROOT,'archive ignores a late response'),
 ('cli-legacy-route','server/cmd/keryx/cmd_correspondence_cases.go','/correspondence/cases','/messengers/inbox',go,ROOT/'server','TestPersonalCorrespondenceUsesCases'),
]:
 source=ROOT/path;original=source.read_bytes();assert old in original.decode(),name
 digest=hashlib.sha256(original).hexdigest()
 try:
  source.write_text(original.decode().replace(old,new,1))
  red=subprocess.run(cmd,cwd=cwd,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
  (OUT/(name+'-red.log')).write_text(red.stdout)
  assert red.returncode!=0 and expected in red.stdout,name+' did not fail named assertion'
 finally:source.write_bytes(original)
 assert hashlib.sha256(source.read_bytes()).hexdigest()==digest
 green=subprocess.run(cmd,cwd=cwd,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
 (OUT/(name+'-restored.log')).write_text(green.stdout);assert green.returncode==0,green.stdout
 results.append(dict(name=name,source=path,red=True,restored=True,sha256=digest))
(OUT/'mutations.json').write_text(json.dumps(results,indent=2))
print('Three named source mutants RED, exact restore GREEN')
