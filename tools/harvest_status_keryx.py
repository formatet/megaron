#!/usr/bin/env python3
"""Run built Keryx against the explicit Mycenae browser HTTP fixture.
Usage: python3 tools/harvest_status_keryx.py /path/to/built/keryx
"""
from pathlib import Path
import http.server,json,os,subprocess,sys,tempfile,threading
ROOT=Path(__file__).resolve().parents[1];OUT=ROOT/'docs/reviews/skordegava-syns'
f=json.loads((OUT/'browser/fixture.json').read_text());active=True;requests=[]
class Handler(http.server.BaseHTTPRequestHandler):
 def log_message(self,*args):pass
 def do_GET(self):
  requests.append(self.path)
  if self.path.endswith('/provinces/'+f['province']):data={'settlement':dict(f['settlement'],harvest_blessing=f['settlement']['harvest_blessing'] if active else None)}
  elif self.path.endswith('/placement-options'):data=f['placement_options']
  elif self.path.endswith('/players/me'):data={'username':'Agamemnon'}
  elif self.path.endswith('/actions'):data=[]
  else:data={}
  body=json.dumps(data).encode();self.send_response(200);self.send_header('Content-Type','application/json');self.end_headers();self.wfile.write(body)
server=http.server.ThreadingHTTPServer(('127.0.0.1',0),Handler);threading.Thread(target=server.serve_forever,daemon=True).start()
try:
 with tempfile.TemporaryDirectory(prefix='harvest-keryx-') as tmp:
  config=Path(tmp)/'config.json';config.write_text(json.dumps(dict(server=f'http://127.0.0.1:{server.server_port}',token='explicit-http-fixture',world_id=f['world'],province_id=f['province'],username='Agamemnon')));config.chmod(0o600)
  for active in [True,False]:
   for cmd in ['status','city']:
    r=subprocess.run([str(Path(sys.argv[1]).resolve()),cmd],env={**os.environ,'POLEIA_CONFIG':str(config)},capture_output=True,text=True,check=True)
    label=cmd+('-active' if active else '-end');(OUT/('keryx-'+label+'.txt')).write_text(r.stdout)
    assert ('Blessed harvest:' in r.stdout)==active,(label,r.stdout)
    if active:
     b=f['settlement']['harvest_blessing'];want=f'Blessed harvest: +{b["yield_bonus"]*100:g}% '+', '.join(b['goods'])+f' yield · {b["remaining_ticks"]} days left'
     assert want in r.stdout,(label,want,r.stdout)
     assert 'fish' not in next(x for x in r.stdout.splitlines() if 'Blessed harvest' in x)
finally:server.shutdown()
(OUT/'keryx-requests.json').write_text(json.dumps(requests,indent=2)+'\n');print('Built Keryx status/city active/end: PASS (explicit HTTP fixture)')
