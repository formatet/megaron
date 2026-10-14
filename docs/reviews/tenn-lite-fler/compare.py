#!/usr/bin/env python3
"""Compare actual production GenerateMap to the unchanged baseline and selected measured candidate."""
import hashlib,json,os,pathlib,subprocess,tempfile
HERE=pathlib.Path(__file__).resolve().parent;ROOT=HERE.parents[2];BASE='2be784c2'
clean={k:os.environ[k] for k in ('HOME','PATH')}
with tempfile.TemporaryDirectory(prefix='tin-production-') as tmp:
 module=pathlib.Path(tmp);pkg=module/'internal/world';pkg.mkdir(parents=True)
 for f in ('go.mod','go.sum'):(module/f).write_bytes((ROOT/'server'/f).read_bytes())
 for path in (ROOT/'server/internal/world').glob('*.go'):
  if not path.name.endswith('_test.go'):(pkg/path.name).write_bytes(path.read_bytes())
 (pkg/'probe_validate.go').write_text('package world\nfunc ProbeValidate(tiles []MapTile,w,h int)error{return validateMap(tiles,w,h)}\n')
 cmd=module/'cmd/probe';cmd.mkdir(parents=True)
 source=(HERE/'probe.go.txt').read_text().replace('land:=world.LandComponents(tiles);','if err:=world.ProbeValidate(tiles,size,size);err!=nil{panic(err)};land:=world.LandComponents(tiles);')
 (cmd/'main.go').write_text(source)
 with (HERE/'production.jsonl').open('wb') as out,(HERE/'production-mapgen.log').open('wb') as log:
  subprocess.run(['go','run','./cmd/probe'],cwd=module,env=clean,stdout=out,stderr=log,check=True)
base=[json.loads(s) for s in (HERE/'baseline.jsonl').read_text().splitlines()]
chosen=[json.loads(s) for s in (HERE/'post2.jsonl').read_text().splitlines()]
actual=[json.loads(s) for s in (HERE/'production.jsonl').read_text().splitlines()];assert len(actual)==len(base)==len(chosen)==40
for old,selected,new in zip(base,chosen,actual):
 assert selected==new,'production differs from pre-implementation measurement'
 assert old['effective_seed']==new['effective_seed'] and old['non_tin_sha256']==new['non_tin_sha256'],'geography changed'
 assert set(map(tuple,old['tin']))<=set(map(tuple,new['tin'])),'old tin moved'
 assert new['tin_hexes']>=old['tin_hexes'],'tin shrank'
summary={'baseline':BASE,'map_count':40,'effective_seeds_preserved':40,'all_non_tin_fields_preserved':40,'old_tin_preserved':40,'actual_validateMap_passed':40,'production_equals_selected_measurement':40,
 'production_diff_sha256':hashlib.sha256(subprocess.check_output(['git','-C',str(ROOT),'diff',BASE,'--','server','web'])).hexdigest()}
(HERE/'comparison.json').write_text(json.dumps(summary,indent=2)+'\n');print(json.dumps(summary,indent=2))
