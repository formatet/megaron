#!/usr/bin/env python3
"""Fresh-PG physical mutations of each terrain's rate and rule; exact restore."""
from pathlib import Path
import hashlib,json,subprocess
HERE=Path(__file__).resolve().parent;ROOT=HERE.parents[2]
SQL=ROOT/'server/db/migrations/189_sparse_timber.up.sql';GO=ROOT/'server/internal/economy/hex_rules.go'
original={p:p.read_bytes() for p in [SQL,GO]}
terrains=['scrub_maquis','hills','river_valley']
rule_lines={
 'scrub_maquis':(b'case "scrub_maquis":\n\t\tout = append(out, fieldStone, hexRule{"timber", 1, "lumbermill"})',b'case "scrub_maquis":\n\t\tout = append(out, fieldStone)'),
 'hills':(b'hexRule{GoodGrain, 1, "farm"}, hexRule{"stone", 2, "stonequarry"}, hexRule{"timber", 1, "lumbermill"}',b'hexRule{GoodGrain, 1, "farm"}, hexRule{"stone", 2, "stonequarry"}'),
 'river_valley':(b'hexRule{GoodGrain, 2, "farm"}, fieldStone, hexRule{"timber", 1, "lumbermill"}',b'hexRule{GoodGrain, 2, "farm"}, fieldStone'),
}
results=[];(HERE/'mutation-logs').mkdir(exist_ok=True)
def run(terrain,label,red):
 expr='^TestSparseTimber_HTTPPlacementAndLumbermill$/^'+terrain+'$'
 proc=subprocess.run([str(ROOT/'tools/gotest.sh'),'./api/handlers','-run',expr],cwd=ROOT,capture_output=True,text=True)
 (HERE/'mutation-logs'/(label+'.log')).write_text(proc.stdout+proc.stderr)
 if red:
  assert proc.returncode!=0 and ('--- FAIL: TestSparseTimber_HTTPPlacementAndLumbermill/'+terrain) in proc.stdout,(label,proc.stdout,proc.stderr)
 else:assert proc.returncode==0,(label,proc.stdout,proc.stderr)
for terrain in terrains:
 for kind,path in [('rate',SQL),('rule',GO)]:
  label=terrain+'-'+kind
  try:
   if kind=='rate':
    old=b"ARRAY['scrub_maquis','hills','river_valley']"
    new=("ARRAY["+','.join("'"+t+"'" for t in terrains if t!=terrain)+']').encode()
   else:old,new=rule_lines[terrain]
   assert old in original[path]
   path.write_bytes(original[path].replace(old,new,1))
   run(terrain,label+'-RED',True)
  finally:
   for p,content in original.items():p.write_bytes(content);assert p.read_bytes()==content
  run(terrain,label+'-restored-GREEN',False)
  results.append(dict(terrain=terrain,mutation=kind,result='named RED → byte-exact restore → fresh GREEN'))
(HERE/'mutations.json').write_text(json.dumps(dict(source_sha256={str(p.relative_to(ROOT)):hashlib.sha256(v).hexdigest() for p,v in original.items()},results=results),ensure_ascii=False,indent=2)+'\n')
print(json.dumps(results,ensure_ascii=False))
