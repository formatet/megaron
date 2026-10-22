#!/usr/bin/env python3
"""Allow rumour coordinates in both consumers, prove named RED, restore exact bytes."""
from pathlib import Path
import os,subprocess
root=Path(__file__).resolve().parents[3];out=Path(__file__).parent
env={k:os.environ[k] for k in ('HOME','PATH')}
for label,path,old,new,cmd,expected in [
 ('cli','server/cmd/keryx/cmd_city_target.go','city.Knowledge == "known"','city.Knowledge != ""',['go','test','-count=1','./cmd/keryx/','-run','^TestCityTargetsCommandAndKnowledge$'],'TestCityTargetsCommandAndKnowledge/march/rumour'),
 ('web','web/static/js/megaron/ui/city_targets.js',"c.knowledge === 'known'","c.knowledge !== ''",['node','--test','web/static/js/megaron/ui/city_targets.test.mjs'],'city shortcut uses only exact Known entries')]:
 p=root/path;original=p.read_bytes();assert original.count(old.encode())==1
 cwd=root/'server' if label=='cli' else root
 try:
  p.write_bytes(original.replace(old.encode(),new.encode()))
  result=subprocess.run(cmd,cwd=cwd,env=env,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
  (out/f'mutation-{label}-red.log').write_bytes(result.stdout)
  assert result.returncode!=0 and expected.encode() in result.stdout,result.stdout.decode()
 finally:p.write_bytes(original)
 assert p.read_bytes()==original
 result=subprocess.run(cmd,cwd=cwd,env=env,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
 (out/f'mutation-{label}-restored.log').write_bytes(result.stdout)
 assert result.returncode==0,result.stdout.decode()
 print(label+': rumour-target mutant named RED; exact-byte restore GREEN')
