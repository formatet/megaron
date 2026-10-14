#!/usr/bin/env python3
"""Old grace60 must fail independent approved-boundary and real join-archive tests."""
import pathlib,os,subprocess
HERE=pathlib.Path(__file__).resolve().parent;ROOT=HERE.parents[2];p=ROOT/'server/internal/kharis/tick.go'
original=p.read_bytes();needle=b'const foundingGraceTicks = 144';assert original.count(needle)==1
cmd=[str(ROOT/'tools/gotest.sh'),'./internal/kharis','./api/handlers','-run','TestShouldRollPunishment_Grace144|TestOpeningDispatchJoinConcurrentRetryRiseAndPrivacy','-v'];clean={k:os.environ[k] for k in ('HOME','PATH')}
try:
 p.write_bytes(original.replace(needle,b'const foundingGraceTicks = 60'))
 r=subprocess.run(cmd,cwd=ROOT,env=clean,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True)
 (HERE/'grace60-red.log').write_text(r.stdout)
 assert r.returncode!=0 and '--- FAIL: TestShouldRollPunishment_Grace144' in r.stdout and '--- FAIL: TestOpeningDispatchJoinConcurrentRetryRiseAndPrivacy' in r.stdout and '[build failed]' not in r.stdout,r.stdout
finally:p.write_bytes(original)
assert p.read_bytes()==original
r=subprocess.run(cmd,cwd=ROOT,env=clean,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True)
(HERE/'grace144-restored-green.log').write_text(r.stdout);assert r.returncode==0,r.stdout
print('PASS60: boundary + real HTTP join archive named RED -> exact bytes -> fresh GREEN')
