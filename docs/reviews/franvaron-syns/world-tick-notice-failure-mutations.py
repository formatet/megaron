#!/usr/bin/env python3
"""A failed notice may never cancel the next world tick: physical return mutations."""
import hashlib
import os
import pathlib
import socket
import subprocess
root=pathlib.Path(__file__).resolve().parents[3]
logs=pathlib.Path(__file__).resolve().parent
path=root/'server/internal/kharis/tick.go'
cases=[
 ('wanax-return','slog.Error("kharis maintenance failed", "player", w.playerID, "world", e.WorldID, "err", err)','return fmt.Errorf("kharis maintenance player %s: %w", w.playerID, err)'),
 ('garrison-return','slog.Error("garrison starvation observation failed", "world", e.WorldID, "err", err)','return err'),
]
test='TestTickNoticeFailureKeepsWorldRunning'
for name,old,new in cases:
 original=path.read_bytes()
 assert original.count(old.encode())==1,(name,'ambiguous mutation')
 def run(phase):
  with socket.socket() as listener:
   listener.bind(('127.0.0.1',0));port=listener.getsockname()[1]
  result=subprocess.run([str(root/'tools/gotest.sh'),'./internal/kharis/','-run','^'+test+'$'],cwd=root,env=dict(os.environ,GOTEST_PORT=str(port)),text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
  (logs/('world-tick-mutation-'+name+'-'+phase+'.log')).write_text(result.stdout)
  assert 'färsk DB på migration 187' in result.stdout,(name,'missing fresh187')
  return result
 try:
  path.write_bytes(original.replace(old.encode(),new.encode()))
  result=run('red')
  assert result.returncode!=0 and '--- FAIL: '+test in result.stdout,(name,'not named RED')
 finally:
  path.write_bytes(original)
 assert path.read_bytes()==original,(name,'restoration mismatch')
 result=run('restored-green')
 assert result.returncode==0,(name,'restored failure')
 print(name+': named RED -> identical SHA256 '+hashlib.sha256(original).hexdigest()+' -> fresh187 GREEN',flush=True)
