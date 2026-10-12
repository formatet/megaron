#!/usr/bin/env python3
"""Build the current place-naming slice in the isolated acceptance runtime.
Uses the current cached acceptance runtime; CGO=0 Go binary and migrations are
rebuilt/copied from this worktree. No DB reset. All source/provenance recorded.
"""
import hashlib,json,os,pathlib,shutil,subprocess,tempfile,time,urllib.request
ROOT=pathlib.Path(__file__).resolve().parents[1]
OUT=pathlib.Path(os.environ.get('PLACE_PROOF_OUTPUT',str(ROOT/'docs/reviews/flod-doparratt')));OUT.mkdir(parents=True,exist_ok=True)
def run(args,**kw):return subprocess.run(args,check=True,**kw)
commit=subprocess.check_output(['git','-C',str(ROOT),'rev-parse','--short=7','HEAD'],text=True).strip()
base=subprocess.check_output(['docker','image','inspect','-f','{{.Id}}','megaron-acc-server'],text=True).strip()
with tempfile.TemporaryDirectory(prefix='parity-runtime-') as tmp:
 tmp=pathlib.Path(tmp);env=dict(os.environ,CGO_ENABLED='0',GOOS='linux')
 run(['go','build','-ldflags','-s -w -X main.buildCommit='+commit,'-o',str(tmp/'temenos'),'./cmd/server'],cwd=ROOT/'server',env=env)
 shutil.copytree(ROOT/'server/db',tmp/'db')
 (tmp/'Dockerfile').write_text(f'FROM {base}\nCOPY temenos /app/temenos\nCOPY db /app/db\n')
 (tmp/'override.json').write_text(json.dumps({'services':{'server':{'build':{'context':str(tmp),'dockerfile':'Dockerfile'}}}}))
 env=dict(os.environ,ACC_TICK_SECONDS='6',ACC_MAP_WIDTH='80',ACC_MAP_HEIGHT='60',BUILD_COMMIT=commit)
 run(['docker','compose','-p','megaron-acc','-f',str(ROOT/'docker-compose.yml'),'-f',str(ROOT/'docker-compose.acceptance.yml'),'-f',str(tmp/'override.json'),'up','-d','--build'],env=env)
 for _ in range(40):
  try:
   health=json.load(urllib.request.urlopen('http://localhost:8097/healthz',timeout=2))
   if health['commit']==commit:break
  except Exception:pass
  time.sleep(.5)
 else:raise RuntimeError('new process did not become healthy')
 latest=max(int(p.name.split('_')[0]) for p in (ROOT/'server/db/migrations').glob('*.up.sql'));assert health['migration']==latest,health
 assets={}
 for rel in ['js/megaron/render/map.js','js/megaron/render/place_names.js','js/megaron/ui/placename.js','megaron.css']:
  local=ROOT/'web/static'/rel
  if not local.exists():continue
  served=urllib.request.urlopen('http://localhost:8097/static/'+rel).read();assert served==local.read_bytes(),rel
  assets[rel]=hashlib.sha256(served).hexdigest()
 (OUT/'runtime.json').write_text(json.dumps(dict(commit=commit,health=health,runtime_base=base,binary_sha256=hashlib.sha256((tmp/'temenos').read_bytes()).hexdigest(),assets=assets,build='CGO=0 local Go; cached acceptance runtime; current migrations; current worktree web mount'),indent=2)+'\n')
run(['tools/acceptance.sh','provenance'],cwd=ROOT,stdout=(OUT/'provenance.txt').open('w'))
print('current process and branch assets verified')
