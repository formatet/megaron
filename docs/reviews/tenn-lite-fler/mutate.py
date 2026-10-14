#!/usr/bin/env python3
"""Disable the additional pass: actual old tin generation must fail the golden-map test."""
import os,pathlib,subprocess
HERE=pathlib.Path(__file__).resolve().parent;ROOT=HERE.parents[2];path=ROOT/'server/internal/world/mapgen.go'
original=path.read_bytes();source=original.decode();old='addTinSources(tiles, eff, width, height)';assert source.count(old)==1
command=[str(ROOT/'tools/gotest.sh'),'./internal/world','-run','^TestGenerateMap_TinIncreasePreservesGeography$','-v']
clean={k:os.environ[k] for k in ('HOME','PATH')}
try:
 path.write_text(source.replace(old,'// mutation: use the unchanged legacy tin map'))
 result=subprocess.run(command,cwd=ROOT,env=clean,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
 (HERE/'old-tin-red.log').write_text(result.stdout)
 assert result.returncode!=0 and '--- FAIL: TestGenerateMap_TinIncreasePreservesGeography' in result.stdout and '[build failed]' not in result.stdout,result.stdout
finally:path.write_bytes(original)
assert path.read_bytes()==original
result=subprocess.run(command,cwd=ROOT,env=clean,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
(HERE/'old-tin-restored-green.log').write_text(result.stdout);assert result.returncode==0,result.stdout
print('PASS actual old tin generation: named RED -> identical bytes -> fresh GREEN')
