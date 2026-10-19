#!/usr/bin/env python3
"""Read-only owner-authorized dev snapshots. Never copies credentials to artifacts."""
from pathlib import Path
import datetime,json,urllib.request
HERE=Path(__file__).resolve().parent
rows=[]
for account,names in [('gpt-idomeneus',['MalliaWest','Mallia']),('luna-pylaimenes',['Phaistos'])]:
 config=json.loads((Path.home()/f'.config/megaron-ai/{account}.json').read_text())
 def api(path):
  req=urllib.request.Request(config['server']+path,headers={'Authorization':'Bearer '+config['token']})
  with urllib.request.urlopen(req,timeout=30) as response:return json.load(response)
 health=api('/healthz');world='/api/v1/worlds/'+config['world_id']
 cities=api(world+'/settlements')
 for name in names:
  city=next(s for s in cities if s['name']==name)
  payload=api(world+'/provinces/'+city['province_id'])
  rows.append(dict(name=name,account=account,health=health,captured_at=datetime.datetime.now().astimezone().isoformat(),world_id=config['world_id'],province_id=city['province_id'],payload=payload))
(HERE/'dev-snapshots.json').write_text(json.dumps(rows,ensure_ascii=False,indent=2)+'\n')
print([(r['name'],r['payload']['settlement']['population'],r['health']) for r in rows])
