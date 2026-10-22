#!/usr/bin/env python3
"""Restore departure-based Host projections in Go and web; exact-byte restore."""
from pathlib import Path
import os,subprocess
root=Path(__file__).resolve().parents[3];out=Path(__file__).parent
for label,path,old,new,cmd,expected in [
 ('server','server/api/handlers/host_position.go','host.Position = row.Physical(ctx, db, worldID, now, anchor.MilliAt(now))','host.Position = province.MapPosition{Q:row.Q,R:row.R}',['tools/gotest.sh','./api/handlers/','-run','^TestHostPhysicalPositionStatusEyesAndFounding$','-v'],'--- FAIL: TestHostPhysicalPositionStatusEyesAndFounding'),
 ('web','web/static/js/megaron/ui/host_state.js','return states.sort','if (host) return {...host.host_state,q:host.q,r:host.r};\n return states.sort',['node','--test','web/static/js/megaron/ui/host_state.test.mjs'],'Host uses physical server projection')]:
 p=root/path;original=p.read_bytes();assert original.count(old.encode())==1
 try:
  p.write_bytes(original.replace(old.encode(),new.encode()))
  result=subprocess.run(cmd,cwd=root,env=dict(os.environ,GOTEST_PORT='57900'),stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
  (out/f'mutation-{label}-red.log').write_bytes(result.stdout)
  assert result.returncode!=0 and expected.encode() in result.stdout,result.stdout.decode()
 finally:p.write_bytes(original)
 assert p.read_bytes()==original
 result=subprocess.run(cmd,cwd=root,env=dict(os.environ,GOTEST_PORT='57900'),stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
 (out/f'mutation-{label}-restored.log').write_bytes(result.stdout)
 assert result.returncode==0,result.stdout.decode()
 print(label+': departure-position mutant named RED; exact-byte restored GREEN')
