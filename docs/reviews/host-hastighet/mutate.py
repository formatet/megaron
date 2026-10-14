#!/usr/bin/env python3
"""Restore the old Host factor physically; require the real HTTP test to fail."""
import os,pathlib,subprocess
HERE=pathlib.Path(__file__).resolve().parent;ROOT=HERE.parents[2]
p=ROOT/'server/internal/unit/model.go';original=p.read_bytes()
needle=b'return 1 / hostSpeedOfSpearman';assert original.count(needle)==1
cmd=[str(ROOT/'tools/gotest.sh'),'./api/handlers','-run','^TestHostSpeed_HTTPPreviewDispatchAndStoredMarch$','-v']
clean={k:os.environ[k] for k in ('HOME','PATH')}
try:
 p.write_bytes(original.replace(needle,b'return 2.0'))
 r=subprocess.run(cmd,cwd=ROOT,env=clean,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
 (HERE/'factor-2-red.log').write_text(r.stdout)
 assert r.returncode!=0 and '--- FAIL: TestHostSpeed_HTTPPreviewDispatchAndStoredMarch' in r.stdout and '[build failed]' not in r.stdout,r.stdout
finally:p.write_bytes(original)
assert p.read_bytes()==original
r=subprocess.run(cmd,cwd=ROOT,env=clean,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
(HERE/'factor-restored-green.log').write_text(r.stdout);assert r.returncode==0,r.stdout
print('PASS old factor 2.0: named HTTP RED -> exact original bytes -> fresh GREEN')
