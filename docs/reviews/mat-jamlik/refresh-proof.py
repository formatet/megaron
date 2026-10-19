#!/usr/bin/env python3
"""Refresh the review manifest from completed evidence, excluding ignored secrets/binaries."""
from pathlib import Path
import hashlib,json,re,subprocess
HERE=Path(__file__).resolve().parent;ROOT=HERE.parents[2]
checks=json.loads((HERE/'checks.json').read_text())
log=(HERE/'variety-share-full-go.log').read_text()
world=re.search(r'ok\s+formatet/megaron/server/internal/world\s+([\d.]+)s',log)
assert world and not re.search(r'^FAIL',log,re.M), 'full fresh suite incomplete or failed'
assert 'färsk DB på migration 190' in log
assert (HERE/'variety-share-vet.log').read_text()==''
assert '# pass 668' in (HERE/'variety-share-js.log').read_text()
mutations=json.loads((HERE/'mutations.json').read_text())
assert len(mutations)==7
for mutation in mutations:
 assert hashlib.sha256((ROOT/mutation['source']).read_bytes()).hexdigest()==mutation['sha256'],mutation
proof=json.loads((HERE/'native-j/proof.json').read_text())
assert not proof['page_errors'] and not proof['sql_writes'] and not proof['api_replay']
checks['production_commit']=proof['health']['commit']
checks['mutations']=mutations
checks['variety_share_review_fix']={'commit':proof['health']['commit'],'full_fresh190_go_exit':0,'world_seconds':float(world[1]),'vet_exit':0,'js_pass':668,'real_share_cases':['one wine among thousand grain: no bonus','99/901: no bonus','100/900: bonus','300/700: bonus']}
checks['natural_proofs']=[p for p in checks['natural_proofs'] if p['commit']!=proof['health']['commit']]+[dict(commit=proof['health']['commit'],migration=proof['health']['migration'],meal_tick=proof['meal_tick'],return_status=proof['return_status'])]
(HERE/'checks.json').write_text(json.dumps(checks,indent=2)+'\n')
files=subprocess.check_output(['git','-C',str(ROOT),'ls-files','--cached','--others','--exclude-standard','-z','--',str(HERE.relative_to(ROOT))]).decode().split('\0')
manifest={}
for name in sorted(set(files)):
 if not name:continue
 path=ROOT/name
 if path.name=='artifact-sha256.json':continue
 manifest[str(path.relative_to(HERE))]=hashlib.sha256(path.read_bytes()).hexdigest()
(HERE/'artifact-sha256.json').write_text(json.dumps(manifest,indent=2)+'\n')
print('Verified seven current-source mutations, fresh Go/vet/JS and native J; refreshed',len(manifest),'artifact hashes')
