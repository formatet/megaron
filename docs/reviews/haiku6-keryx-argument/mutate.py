#!/usr/bin/env python3
from pathlib import Path
import subprocess,json,os
HERE=Path(__file__).resolve().parent;ROOT=HERE.parents[2]
CLEAN={key:os.environ[key] for key in ["HOME","PATH"]}
cases=[
 ('build','cmd_build.go','len(args) > 0 && buildingType == "" && !list && !queue','false','building_mistaken_for_city'),
 ('staff','cmd_staff.go','if len(args) == 0 {','if false {','missing_workplace'),
 ('source','cmd_goods.go','src := cfg.ProvinceID','src := "capital-province"','TestHaiku6TransferSelectedColonyAndExplicitSource'),
]
# The quote mutation is an early return, leaving the genuine quoting code in place.
cases.append(('quote','cmd_args.go','func commandArgument(arg string) string {','func commandArgument(arg string) string {\n return arg','TestSuggestedArgumentSurvivesShellCopy'))
results=[]
for name,file,old,new,expected in cases:
 p=ROOT/'server/cmd/keryx'/file;saved=p.read_bytes();assert saved.count(old.encode())==1,(name,old)
 try:
  p.write_bytes(saved.replace(old.encode(),new.encode()))
  r=subprocess.run(['go','-C',str(ROOT/'server'),'test','./cmd/keryx','-run','TestHaiku6|TestSuggestedArgumentSurvivesShellCopy','-v'],capture_output=True,text=True,env=CLEAN)
  assert r.returncode!=0 and ('FAIL: '+expected in r.stdout or expected in r.stdout and '--- FAIL:' in r.stdout),r.stdout+r.stderr
  results.append(dict(mutation=name,red=r.stdout+r.stderr,exit_code=r.returncode))
 finally:p.write_bytes(saved)
 assert p.read_bytes()==saved
 r=subprocess.run(['go','-C',str(ROOT/'server'),'test','./cmd/keryx','-run','TestHaiku6|TestSuggestedArgumentSurvivesShellCopy'],capture_output=True,text=True,env=CLEAN);assert r.returncode==0,r.stdout+r.stderr
 results[-1]['restore_green']=r.stdout+r.stderr
(HERE/'mutations.json').write_text(json.dumps(results,indent=2)+'\n');print('Four named RED → exact restore → GREEN; no persistent DB used by CLI HTTP contracts')
