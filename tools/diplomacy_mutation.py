#!/usr/bin/env python3
"""Physical consumer mutations for M; always restore source, require named red."""
from pathlib import Path
import subprocess
ROOT=Path(__file__).resolve().parents[1]
p=ROOT/'web/static/js/megaron/ui/drawers/diplomacy.js'
original=p.read_text();logs=ROOT/'docs/reviews/forenkling-diplomacy/logs';logs.mkdir(parents=True,exist_ok=True)
cases=[
 ('tabs','<button class="dtab" data-tab="known">Known</button>', '<button class="dtab" data-tab="known">Known</button><button class="dtab" data-tab="compose">Compose</button>','actual drawer offers exactly'),
 ('draft','if (draftDestination && writableCities().some(', 'if (false && draftDestination && writableCities().some(', 'first letter and both trade directions'),
 ('rumour','const contacts = new Map(writableCities().map(p => [p.settlement_id, p]));', 'const contacts = new Map(cities.map(p => [p.settlement_id, p]));','Known keeps ruler and city knowledge'),
 ('draft-time', "(msgs.length ? fmtAgo(latest.arrived_at || latest.sent_at || latest.created_at) : 'New conversation')", "fmtAgo(latest.arrived_at || latest.sent_at || latest.created_at)", 'first letter and both trade directions'),
 ('words','${numberWords(r.known_cities)} known', '${r.known_cities} known','Known keeps ruler and city knowledge'),
]
for name,old,new,named in cases:
 assert original.count(old)==1,(name,original.count(old))
 try:
  p.write_text(original.replace(old,new))
  r=subprocess.run(['node','--test','web/static/js/megaron/ui/drawers/diplomacy_simple.test.mjs'],cwd=ROOT,text=True,capture_output=True)
  (logs/(name+'-red.log')).write_text(r.stdout+r.stderr)
  assert r.returncode!=0 and any('not ok' in line and named in line for line in r.stdout.splitlines()),(name,r.stdout)
 finally:p.write_text(original)
 r=subprocess.run(['node','--test','web/static/js/megaron/ui/drawers/diplomacy_simple.test.mjs'],cwd=ROOT,text=True,capture_output=True)
 (logs/(name+'-restored.log')).write_text(r.stdout+r.stderr);assert r.returncode==0,r.stdout
 print(name+': named red → restored green')
