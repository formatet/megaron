#!/usr/bin/env python3
"""Render a genuine, frozen rig API response through both built CLI versions.
No authentication or database writes; the local server only serves this snapshot.
Usage: replay.py /path/to/before-keryx /path/to/after-keryx
"""
import http.server,json,os,subprocess,sys,tempfile,threading
from pathlib import Path
proof=Path(__file__).resolve().parent
snapshot=(proof/'rigg-notifications.json').read_bytes()
class Replay(http.server.BaseHTTPRequestHandler):
 def do_GET(self):
  if self.path.startswith('/api/v1/worlds/rig-world/notifications'):
   payload=snapshot
  elif self.path=='/api/v1/worlds/rig-world':
   payload=b'{"id":"rig-world","tick_seconds":6}'
  else:
   self.send_error(404);return
  self.send_response(200);self.send_header('Content-Type','application/json');self.end_headers();self.wfile.write(payload)
 def log_message(self,*args):pass
server=http.server.ThreadingHTTPServer(('127.0.0.1',0),Replay)
thread=threading.Thread(target=server.serve_forever,daemon=True);thread.start()
try:
 with tempfile.TemporaryDirectory() as tmp:
  cfg=Path(tmp)/'config.json';cfg.write_text(json.dumps({'server':f'http://127.0.0.1:{server.server_port}','world_id':'rig-world'}))
  env={k:v for k,v in os.environ.items() if not k.startswith('POLEIA_')};env['POLEIA_CONFIG']=str(cfg)
  for label,binary in zip(('before','after'),sys.argv[1:]):
   for mode,args in (('human',[]),('json',['--json'])):
    result=subprocess.run([binary,*args,'notifications','--exclude','none'],env=env,capture_output=True,text=True,check=True)
    (proof/f'rigg-{label}-{mode}.txt').write_text(result.stdout)
    assert not result.stderr,result.stderr
    if mode=='json':assert json.loads(result.stdout)==json.loads(snapshot)
  after=(proof/'rigg-after-human.txt').read_text()
  assert '"settlement_id"' not in after and '{"' not in after
  assert 'Your metropolis is founded: Mycenae' in after
  print(f'{len(json.loads(snapshot)["notifications"])} real notifications; identical input; both --json outputs preserve the entire response')
finally:server.shutdown();server.server_close()
