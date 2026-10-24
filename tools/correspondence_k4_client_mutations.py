#!/usr/bin/env python3
"""K2 client mutants: stable IDs, private-archive route, stale world response.
Run in the owned K2 worktree; restores bytes after each mutant.
"""
from pathlib import Path
import hashlib,json,subprocess
ROOT=Path(__file__).resolve().parents[1]
OUT=ROOT/'docs/reviews/correspondence-k4/client'
OUT.mkdir(parents=True,exist_ok=True)
results=[]
js=['node','--test','web/static/js/megaron/ui/drawers/diplomacy_archive.test.mjs']
go=['go','test','./cmd/keryx','-run','TestPersonalCorrespondence','-count=1']
for name,path,old,new,cmd,cwd,expected in [
 ('private-quote-as-common-promise','web/static/js/megaron/ui/drawers/diplomacy_memory.js','My note ·','Shared promise ·',['node','--test','web/static/js/megaron/ui/drawers/diplomacy_memory.test.mjs'],ROOT,'memory separates accepted engine terms'),
 ('wrong-citation-source','web/static/js/megaron/ui/drawers/diplomacy_memory.js',"source = form.elements.source.value", "source = 'message'",['node','--test','web/static/js/megaron/ui/drawers/diplomacy_memory.test.mjs'],ROOT,'keeping a private passage sends'),
 ('memory-late-case','web/static/js/megaron/ui/drawers/diplomacy_memory.js','if (!alive() || n !== generation) return;','if (false) return;',['node','--test','web/static/js/megaron/ui/drawers/diplomacy_memory.test.mjs'],ROOT,'memory loader uses same case'),
 ('source-link-drops-letter','web/static/js/megaron/ui/drawers/diplomacy_memory.js','openCase(button.dataset.memoryCase, button.dataset.memoryLetter)','openCase(button.dataset.memoryCase)',['node','--test','web/static/js/megaron/ui/drawers/diplomacy_memory.test.mjs'],ROOT,'memory source link carries exact'),
 ('cli-source-wrong-route','server/cmd/keryx/cmd_correspondence_cases.go','"/letters/"','"/wrong/"',['go','test','./cmd/keryx','-run','TestCorrespondenceExactSourceLetterCommand','-count=1'],ROOT/'server','TestCorrespondenceExactSourceLetterCommand'),
 ('cli-memory-wrong-route','server/cmd/keryx/cmd_correspondence_memory.go','/memory?%s','/inbox?%s',['go','test','./cmd/keryx','-run','TestCorrespondenceMemoryAndCitationCommands','-count=1'],ROOT/'server','TestCorrespondenceMemoryAndCitationCommands'),
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
print('Six named client source mutants RED, exact restore GREEN')
