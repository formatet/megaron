#!/usr/bin/env python3
"""Physical build-error mutant in an isolated detached worktree, fresh DB per arm."""
from pathlib import Path
import tempfile,subprocess,os,hashlib,json
root=Path(__file__).resolve().parents[3];out=Path(__file__).resolve().parent
scratch=Path(tempfile.mkdtemp(prefix='megaron-build-level-mutant-'))
subprocess.run(['git','-C',str(root),'worktree','add','--detach',str(scratch),'HEAD'],check=True,stdout=subprocess.DEVNULL)
path=scratch/'server/api/handlers/province.go';original=path.read_bytes()
try:
 mutant=original.replace(b'writeGoodsError(w, insErr, buildContext)',b'writeGoodsError(w, insErr)',1)
 assert mutant!=original
 path.write_bytes(mutant)
 env={**os.environ,'GOTEST_PORT':'57900'}
 args=[str(scratch/'tools/gotest.sh'),'./api/handlers','-run','^TestBuildShipyardRefusalNamesExistingAndTargetLevels$']
 with (out/'mutant-red.log').open('w') as log:result=subprocess.run(args,cwd=scratch,env=env,stdout=log,stderr=subprocess.STDOUT)
 assert result.returncode!=0 and '--- FAIL: TestBuildShipyardRefusalNamesExistingAndTargetLevels' in (out/'mutant-red.log').read_text()
 path.write_bytes(original)
 subprocess.run(['git','-C',str(scratch),'diff','--exit-code'],check=True,stdout=subprocess.DEVNULL)
 with (out/'mutant-restored-green.log').open('w') as log:result=subprocess.run(args,cwd=scratch,env=env,stdout=log,stderr=subprocess.STDOUT)
 assert result.returncode==0
 assert path.read_bytes()==original==(root/'server/api/handlers/province.go').read_bytes()
 (out/'mutation.json').write_text(json.dumps({'commit':subprocess.check_output(['git','-C',str(root),'rev-parse','HEAD'],text=True).strip(),'mutation':'omit buildContext from the actual Build handler goods refusal','red':True,'exact_restore':True,'fresh_green':True,'file_sha256':hashlib.sha256(original).hexdigest()},indent=2)+'\n')
finally:
 path.write_bytes(original)
 subprocess.run(['git','-C',str(root),'worktree','remove',str(scratch)],check=True)
