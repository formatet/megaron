#!/usr/bin/env python3
"""Measure actual GenerateMap in isolated production source copies; no DB or repo mutations."""
import json,os,pathlib,subprocess,tempfile
HERE=pathlib.Path(__file__).resolve().parent;ROOT=HERE.parents[2];BASE='2be784c2'
clean={k:os.environ[k] for k in ('HOME','PATH')}
def git(*a):return subprocess.check_output(['git','-C',str(ROOT),*a])
variants={'baseline':(2,4),'floor3-cap6':(3,6),'floor4-cap8':(4,8),'area3':None,'post1':'post1','post2':'post2','post3':'post3'}
with tempfile.TemporaryDirectory(prefix='tin-measure-') as tmp:
 for label,params in variants.items():
  module=pathlib.Path(tmp)/label;pkg=module/'internal/world';pkg.mkdir(parents=True)
  for f in ('go.mod','go.sum'):(module/f).write_bytes(git('show',BASE+':server/'+f))
  for name in git('ls-tree','-r','--name-only',BASE,'server/internal/world').decode().splitlines():
   if not name.endswith('.go') or name.endswith('_test.go'):continue
   data=git('show',BASE+':'+name)
   if name.endswith('/mapgen.go'):
    s=data.decode()
    if isinstance(params,str):
     s=s.replace('return tiles, eff','addTinSources(tiles, eff, width, height)\n\t\treturn tiles, eff')
    elif params:
     s=s.replace('tinSourceFloor   = 2','tinSourceFloor   = '+str(params[0])).replace('tinSourceCap     = 4','tinSourceCap     = '+str(params[1]))
    else:
     s=s.replace('tinSourceTarget(players), tinClusterMin','max(3, int(math.Round(3*float64(width*height)/(64*64)))), tinClusterMin')
     s=s.replace('}) < tinSourceCap {','}) < max(3, int(math.Round(3*float64(width*height)/(64*64)))) {')
    data=s.encode()
   (pkg/pathlib.Path(name).name).write_bytes(data)
  if isinstance(params,str):
   (pkg/'extra_tin.go').write_text((HERE/'extra_tin.go.txt').read_text().replace('tinSourcesAddedAtReference = 1','tinSourcesAddedAtReference = '+label[-1]))
  cmd=module/'cmd/probe';cmd.mkdir(parents=True);(cmd/'main.go').write_bytes((HERE/'probe.go.txt').read_bytes())
  with (HERE/(label+'.jsonl')).open('wb') as out,(HERE/(label+'.log')).open('wb') as log:
   subprocess.run(['go','run','./cmd/probe'],cwd=module,env=clean,stdout=out,stderr=log,check=True)
  print('generated '+label,flush=True)
base=[json.loads(s) for s in (HERE/'baseline.jsonl').read_text().splitlines()];summary=[]
for label in variants:
 rows=[json.loads(s) for s in (HERE/(label+'.jsonl')).read_text().splitlines()]
 for size in (64,128):
  pairs=[(a,b) for a,b in zip(base,rows) if a['size']==size]
  mean=lambda key:sum(b[key] for a,b in pairs)/len(pairs)
  old=sum(a['tin_hexes'] for a,b in pairs)/len(pairs)
  summary.append({'variant':label,'size':size,'hexes_mean':mean('tin_hexes'),'sources_mean':mean('tin_sources'),'increase_pct':100*(mean('tin_hexes')/old-1),'effective_seeds_unchanged':sum(a['effective_seed']==b['effective_seed'] for a,b in pairs),'non_tin_fields_unchanged':sum(a['non_tin_sha256']==b['non_tin_sha256'] for a,b in pairs)})
(HERE/'summary.json').write_text(json.dumps(summary,indent=2)+'\n');print(json.dumps(summary,indent=2))
