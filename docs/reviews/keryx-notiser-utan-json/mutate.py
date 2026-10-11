#!/usr/bin/env python3
"""Physical mutations; restore every edited byte even when the test fails."""
from pathlib import Path
import subprocess
root=Path(__file__).resolve().parents[3]
proof=Path(__file__).resolve().parent
text=root/'server/cmd/keryx/notification_text.go'
rows=root/'server/cmd/keryx/cmd_notifications.go'
new=root/'server/internal/notify/mutation_notice.go'
cases=[
 ('missing-kind',text,'case "TrainComplete":','case "TrainCompleteMutation":','TestEveryNotificationKindHasReadableDetail/TrainComplete'),
 ('raw-row',rows,'fmt.Printf("%s[%s]  %s\\n", marker, notificationAge(n.CreatedAt), kind)','fmt.Printf("%s[%s]  %s %s\\n", marker, notificationAge(n.CreatedAt), kind, string(n.Body))','TestEveryNotificationKindHasReadableDetail/TrainComplete'),
 ('raw-group',rows,'fmt.Printf("%s[%s]  %s  ×%d (latest)\\n", marker, notificationAge(latest.CreatedAt), kind, len(occ))','fmt.Printf("%s[%s]  %s  ×%d (latest: %s)\\n", marker, notificationAge(latest.CreatedAt), kind, len(occ), string(latest.Body))','TestNotificationHumanFeedGroupingAndJSON'),
 ('new-server-kind',new,None,'package notify\nimport("context";"github.com/google/uuid")\nfunc mutationNotice(h *Hub){_ = h.NotifyPlayer(context.Background(),uuid.Nil,uuid.Nil,"BrandNewNotice",3,map[string]any{})}\n','TestEveryNotificationKindHasReadableDetail/BrandNewNotice'),
]
for name,path,before,after,test in cases:
 saved=path.read_bytes() if path.exists() else None
 try:
  if before:
   content=saved.decode();assert content.count(before)==1,(name,'mutation anchor');path.write_text(content.replace(before,after))
  else:
   assert saved is None;path.write_text(after)
  result=subprocess.run(['go','test','./cmd/keryx','-count=1','-run','^'+test+'$'],cwd=root/'server',capture_output=True,text=True)
  (proof/('mutation-'+name+'.log')).write_text(result.stdout+result.stderr)
  assert result.returncode!=0,(name,'mutation survived')
  assert 'FAIL: '+test.split('/')[0] in result.stdout,(name,'wrong failure')
  print(name+': named red')
 finally:
  if saved is None:path.unlink(missing_ok=True)
  else:path.write_bytes(saved)
result=subprocess.run(['go','test','./cmd/keryx','-count=1'],cwd=root/'server',capture_output=True,text=True)
(proof/'mutation-restored.log').write_text(result.stdout+result.stderr)
assert result.returncode==0
print('restored: green')
