#!/usr/bin/env python3
"""K2 client mutants: stable IDs, private-archive route, stale world response.
Run in the owned K2 worktree; restores bytes after each mutant.
"""
from pathlib import Path
import hashlib,json,subprocess
ROOT=Path(__file__).resolve().parents[1]
OUT=ROOT/'docs/reviews/correspondence-k3/client'
OUT.mkdir(parents=True,exist_ok=True)
results=[]
js=['node','--test','web/static/js/megaron/ui/drawers/diplomacy_archive.test.mjs']
go=['go','test','./cmd/keryx','-run','TestPersonalCorrespondence','-count=1']
for name,path,old,new,cmd,cwd,expected in [
 ('private-stale-adopts-version','web/static/js/megaron/ui/drawers/diplomacy_private.js',"if (response.status === 409) { status.textContent", "if (response.status === 409) { version = data.current_version; status.textContent",['node','--test','web/static/js/megaron/ui/drawers/diplomacy_private.test.mjs'],ROOT,'private editor keeps local text'),
 ('private-late-case','web/static/js/megaron/ui/drawers/diplomacy_private.js','if (!alive()) return;','if (false) return;', ['node','--test','web/static/js/megaron/ui/drawers/diplomacy_private.test.mjs'],ROOT,'private editor ignores late documents'),
 ('clear-newer-draft','web/static/js/megaron/ui/drawers/diplomacy_private.js',"if (clearSent !== null && draft.value === submittedDraft) draft.value = '';", "if (clearSent !== null) draft.value = '';", ['node','--test','web/static/js/megaron/ui/drawers/diplomacy_private.test.mjs'],ROOT,'successful dispatch clears only'),
 ('cli-wrong-version','server/cmd/keryx/cmd_correspondence_private.go','"expected_version": version','"expected_version": 0',['go','test','./cmd/keryx','-run','TestCorrespondencePrivateReadSaveAndConflict','-count=1'],ROOT/'server','TestCorrespondencePrivateReadSaveAndConflict'),
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
print('Four named client source mutants RED, exact restore GREEN')
