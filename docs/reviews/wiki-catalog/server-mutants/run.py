#!/usr/bin/env python3
"""Run physical catalogue mutants in an isolated CHECKOUT; write proofs to OUTPUT."""
import hashlib,json,os,pathlib,socket,subprocess,sys
root,out=map(pathlib.Path,sys.argv[1:3]);out.mkdir(parents=True,exist_ok=True)
coverage='TestCodexEntityCoverageFromRuntimeCatalogues'
reference='TestCodexReferenceMatchesAuthoritativeCatalogues'
def enum(data):
 # An inferred typed expression, deliberately outside the explicit enum block:
 # the old BasicLit-only collector silently missed this valid catalogue value.
 return data+b'\nconst TypeCodexProbe = Type("codex_probe")\n'
def missing(data):
 obj=json.loads(data)
 for a in obj['articles']:
  a['entities']=[e for e in a.get('entities',[]) if e!={'kind':'building','key':'farm'}]
 return (json.dumps(obj,ensure_ascii=False,indent=2)+'\n').encode()
def replace(old,new):
 def change(data):
  assert data.count(old)==1,(old,data.count(old))
  return data.replace(old,new,1)
 return change
cases=[
 ('inferred-enum-addition','server/internal/unit/model.go',enum,coverage,'current catalogue entities without article'),
 ('missing-index-entity','web/static/codex/index.json',missing,coverage,'building:farm'),
 ('canonical-unit-rename','server/internal/unit/model.go',replace(b'string(TypeSpearman):      "Spearmen"',b'string(TypeSpearman):      "Shieldbearers"'),coverage,'omit canonical unit name'),
 ('runtime-building-cost','server/internal/province/building.go',replace(b'BuildingFarm:        {Costs: map[string]float64{"timber": 6, "stone": 4}',b'BuildingFarm:        {Costs: map[string]float64{"timber": 7, "stone": 4}'),reference,'Codex reference drifted'),
]
def run(name,test):
 with socket.socket() as s:s.bind(('127.0.0.1',0));port=s.getsockname()[1]
 env=os.environ.copy();env['GOTEST_PORT']=str(port)
 p=subprocess.run(['tools/gotest.sh','./api/handlers','-run','^'+test+'$'],cwd=root,env=env,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
 (out/(name+'.log')).write_text(p.stdout);return p
summary=[]
for name,file,change,test,message in cases:
 path=root/file;original=path.read_bytes();mutant=change(original);assert mutant!=original
 try:
  path.write_bytes(mutant);red=run(name+'-red',test)
 finally:path.write_bytes(original)
 assert path.read_bytes()==original
 assert red.returncode!=0 and ('--- FAIL: '+test) in red.stdout and message in red.stdout and 'build failed' not in red.stdout and 'panic:' not in red.stdout,(name,red.stdout[-2200:])
 green=run(name+'-restored-green',test);assert green.returncode==0,(name,green.stdout[-2200:])
 summary.append({'mutant':name,'file':file,'assertion':test,'red_message':message,'assertion_red':True,'exact_restored_sha256':hashlib.sha256(original).hexdigest(),'fresh_restore_green':True})
 (out/'summary.json').write_text(json.dumps(summary,indent=2)+'\n');print(name+': assertion RED, exact restore, fresh GREEN',flush=True)
