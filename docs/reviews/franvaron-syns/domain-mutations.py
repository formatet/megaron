#!/usr/bin/env python3
"""Physical transition mutations: named RED, exact byte restore, fresh PG GREEN."""
import hashlib
import os
import pathlib
import socket
import subprocess
root=pathlib.Path(__file__).resolve().parents[3]
logs=pathlib.Path(__file__).resolve().parent
cases=[
 ('notice-suppressed','if prior == next {','if true {','TestGarrisonTransitionNoticeLifecycle'),
 ('city-witness','e.payload->>\'garrison_settlement_id\'=s.id::text', 'true','TestGarrisonTransitionOwnerAndCityIsolation'),
 ('band-boundary','amount >= religion.MoodIndifferent:', 'amount >= religion.MoodIndifferent+1:', 'TestKharisBandNoticeBoundariesAndExternalChanges'),
]
p=root/'server/internal/kharis/transition_notices.go'
for name,old,new,test in cases:
 original=p.read_bytes()
 assert original.count(old.encode())==1,(name,'ambiguous mutation')
 def run(phase):
  with socket.socket() as listener:
   listener.bind(('127.0.0.1',0));port=listener.getsockname()[1]
  result=subprocess.run([str(root/'tools/gotest.sh'),'./internal/kharis/','-run','^'+test+'$'],cwd=root,env=dict(os.environ,GOTEST_PORT=str(port)),text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
  (logs/('domain-mutation-'+name+'-'+phase+'.log')).write_text(result.stdout)
  assert 'färsk DB på migration 187' in result.stdout,(name,'missing fresh187')
  return result
 try:
  p.write_bytes(original.replace(old.encode(),new.encode()))
  result=run('red')
  assert result.returncode!=0 and '--- FAIL: '+test in result.stdout,(name,'not named RED')
 finally:
  p.write_bytes(original)
 assert p.read_bytes()==original,(name,'restoration mismatch')
 result=run('restored-green')
 assert result.returncode==0,(name,'restored failure')
 print(name+': named RED -> identical SHA256 '+hashlib.sha256(original).hexdigest()+' -> fresh187 GREEN',flush=True)
