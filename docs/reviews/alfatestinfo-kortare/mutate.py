#!/usr/bin/env python3
"""A dropped real screenshot reference must fail the public HTML route test."""
import os,pathlib,subprocess
HERE=pathlib.Path(__file__).resolve().parent;ROOT=HERE.parents[2];path=ROOT/'web/templates/alfatestinfo.html'
original=path.read_bytes();source=original.decode();needle='src="/static/alfatest/produktion.png"';assert source.count(needle)==1
command=[str(ROOT/'tools/gotest.sh'),'./cmd/server','-run','^TestAlfatestInfoPublic$','-v'];clean={k:os.environ[k] for k in ('HOME','PATH')}
try:
 path.write_text(source.replace(needle,'src="/static/alfatest/missing.png"'))
 result=subprocess.run(command,cwd=ROOT,env=clean,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
 (HERE/'missing-image-red.log').write_text(result.stdout)
 assert result.returncode!=0 and '--- FAIL: TestAlfatestInfoPublic' in result.stdout and '[build failed]' not in result.stdout,result.stdout
finally:path.write_bytes(original)
assert path.read_bytes()==original
result=subprocess.run(command,cwd=ROOT,env=clean,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
(HERE/'missing-image-restored-green.log').write_text(result.stdout);assert result.returncode==0,result.stdout
print('PASS missing image reference: named RED -> identical bytes -> fresh GREEN')
