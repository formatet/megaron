#!/usr/bin/env python3
"""Neutralise only the By/category backstop in an isolated clean worktree."""
import hashlib,json,subprocess,sys
from pathlib import Path
root=Path(sys.argv[1]).resolve()
out=Path(__file__).resolve().parent
run=lambda *args:subprocess.run(args,cwd=root,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
assert not run('git','diff','--name-only').stdout.strip(),'mutant worktree must be clean'
path=root/'server/internal/transport/transport.go'
before=path.read_bytes()
old='if (p.By == "land" && p.Category != "land") || (p.By == "sea" && p.Category != "naval") {'
assert before.decode().count(old)==1
try:
 path.write_text(before.decode().replace(old,'if false {',1))
 red=run('tools/gotest.sh','./internal/transport','-run','^TestDispatchLandChoiceRejectsOtherwiseValidNavalTransport$')
 (out/'mutant-red.log').write_text(red.stdout)
 assert red.returncode!=0 and 'explicit land choice dispatched a naval transport' in red.stdout,red.stdout
 assert 'valid sea control refused' not in red.stdout,red.stdout
finally:
 path.write_bytes(before)
assert path.read_bytes()==before
restored=run('tools/gotest.sh','./internal/transport','-run','^TestDispatchLandChoiceRejectsOtherwiseValidNavalTransport$')
(out/'restored-green.log').write_text(restored.stdout)
assert restored.returncode==0,restored.stdout
(out/'mutation.json').write_text(json.dumps({'base':run('git','rev-parse','HEAD').stdout.strip(),'mutation':'prepareJourney explicit By/category guard replaced with if false','test':'TestDispatchLandChoiceRejectsOtherwiseValidNavalTransport/land','assertion':'explicit land choice dispatched a naval transport','red_exit':red.returncode,'restored_exit':restored.returncode,'exact_restored_sha256':hashlib.sha256(before).hexdigest(),'fresh_db_each_arm':True},indent=2)+'\n')
print('PASS: guard mutant assertion-red; exact restore and fresh DB green')
