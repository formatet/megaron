#!/usr/bin/env python3
"""Generate real baseline and branch maps in isolated source copies; compare every
MapTile field except SilverDeposit. Never touches server source or a database.
"""
import hashlib,json,os,pathlib,subprocess,tempfile
HERE=pathlib.Path(__file__).resolve().parent;ROOT=HERE.parents[2];BASE='97712ede'
def run(*args,**kw):return subprocess.check_output(args,**kw)
clean={k:os.environ[k] for k in ('HOME','PATH')}
with tempfile.TemporaryDirectory(prefix='silverhex-compare-') as tmp:
 tmp=pathlib.Path(tmp)
 for label,revision in [('baseline',BASE),('candidate',None)]:
  module=tmp/label;pkg=module/'internal/world';pkg.mkdir(parents=True)
  for f in ['go.mod','go.sum']:
   (module/f).write_bytes(run('git','-C',str(ROOT),'show',BASE+':server/'+f))
  names=run('git','-C',str(ROOT),'ls-tree','-r','--name-only',BASE,'server/internal/world').decode().splitlines()
  for name in names:
   if not name.endswith('.go') or name.endswith('_test.go'):continue
   dest=pkg/pathlib.Path(name).name
   dest.write_bytes(run('git','-C',str(ROOT),'show',revision+':'+name) if revision else (ROOT/name).read_bytes())
  cmd=module/'cmd/probe';cmd.mkdir(parents=True);(cmd/'main.go').write_bytes((HERE/'probe.go.txt').read_bytes())
  with (HERE/(label+'.jsonl')).open('wb') as out,(HERE/(label+'-mapgen.log')).open('wb') as log:
   subprocess.run(['go','run','./cmd/probe'],cwd=module,env=clean,stdout=out,stderr=log,check=True)
a=[json.loads(s) for s in (HERE/'baseline.jsonl').read_text().splitlines()]
b=[json.loads(s) for s in (HERE/'candidate.jsonl').read_text().splitlines()]
assert len(a)==len(b)==20
assert len({m['effective_seed'] for m in b})==20
for old,new in zip(a,b):
 assert old['requested_seed']==new['requested_seed']
 assert old['effective_seed']==new['effective_seed'], 'effective seed changed'
 assert old['all_non_silver_fields_sha256']==new['all_non_silver_fields_sha256'], 'non-silver map field changed'
 assert sum(old['silver_hexes'].values())==4, 'baseline changed'
 assert sum(new['silver_hexes'].values())==10, 'not ten silver hexes'
def capacity(rows):return sum(5*(m['silver_hexes'].get('hills',0)+2.5*m['silver_hexes'].get('mountain_limestone',0)) for m in rows)/len(rows)
result={'baseline_commit':BASE,'tested_production_diff_sha256':hashlib.sha256(run('git','-C',str(ROOT),'diff',BASE,'--','server','web')).hexdigest(),
 'seeds':20,'distinct_effective_seeds':20,'matching_effective_seeds':20,'matching_all_non_silver_map_fields':20,
 'baseline_silver_hexes_each':4,'candidate_silver_hexes_each':10,'baseline_l1_mean':capacity(a),'candidate_l1_mean':capacity(b),
 'candidate_l3_mean':capacity(b)*31/17}
(HERE/'comparison.json').write_text(json.dumps(result,indent=2)+'\n')
print(json.dumps(result,indent=2))
