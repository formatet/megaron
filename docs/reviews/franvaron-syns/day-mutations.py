#!/usr/bin/env python3
"""Physical day/history/WS failures, exact restore, fresh PG green per case."""
from pathlib import Path
import subprocess,os,socket
ROOT=Path(__file__).resolve().parents[3];OUT=Path(__file__).resolve().parent
cases=[
 ('wrong-world','server/db/migrations/187_notification_world_tick.up.sql','SELECT current_tick INTO NEW.world_tick FROM worlds WHERE id = NEW.world_id;','NEW.world_tick := current_world_tick();','./api/handlers','TestNotificationDayBelongsToRecordedWorldAndNeverDrifts'),
 ('delayed-push','server/internal/notify/hub.go','SELECT world_tick FROM notifications WHERE id=$1 AND world_id=$2 AND player_id=$3','SELECT current_tick FROM worlds WHERE id=$2 AND EXISTS(SELECT 1 FROM notifications WHERE id=$1 AND player_id=$3)','./internal/notify','TestNotificationWorldTickSurvivesDelayedDelivery'),
 ('invented-history','server/db/migrations/187_notification_world_tick.up.sql','CREATE FUNCTION capture_notification_world_tick()', 'UPDATE notifications SET world_tick=0 WHERE world_tick IS NULL;\n\nCREATE FUNCTION capture_notification_world_tick()','./api/handlers','TestNotificationDayMigrationKeepsUnknownHistoryUnknown'),
]
def run(label,phase,package,name):
 with socket.socket() as sock:sock.bind(('127.0.0.1',0));port=sock.getsockname()[1]
 env=dict(os.environ,GOTEST_PORT=str(port))
 r=subprocess.run([str(ROOT/'tools/gotest.sh'),package,'-run','^'+name+'$'],cwd=ROOT,env=env,capture_output=True,text=True)
 (OUT/(label+'-'+phase+'.log')).write_text(r.stdout+r.stderr)
 return r,r.stdout+r.stderr
for label,filename,old,new,package,name in cases:
 p=ROOT/filename;original=p.read_bytes();assert original.count(old.encode())==1,label
 try:
  p.write_bytes(original.replace(old.encode(),new.encode(),1));r,log=run(label,'red',package,name)
  assert r.returncode!=0 and '--- FAIL: '+name in log,(label,log)
  assert 'färsk DB på migration 187' in log,(label,log)
 finally:p.write_bytes(original)
 assert p.read_bytes()==original
 r,log=run(label,'restored',package,name);assert r.returncode==0 and 'färsk DB på migration 187' in log,(label,log)
 print(label+': named RED -> exact bytes -> fresh187 GREEN',flush=True)
