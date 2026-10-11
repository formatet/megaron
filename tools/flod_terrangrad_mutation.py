#!/usr/bin/env python3
from pathlib import Path
import subprocess
out=Path('docs/reviews/flod-terrangrad')
cases=[('terrain-row','web/static/js/megaron/render/map.js',"document.getElementById('ip-terrain').textContent = terrainLabel(tile.terrain);", "document.getElementById('ip-terrain').textContent = namedRiverTerrain(tile, terrainLabel(tile.terrain));",['node','--test','web/static/js/megaron/render/inspect_simple.test.mjs'],'river hex and rural panel name'),('keryx-row','server/cmd/keryx/cmd_city.go','h.HexQ, h.HexR, h.Terrain)','h.HexQ, h.HexR, namedRiverTerrain(h.Terrain, h.RiverName))',['go','-C','server','test','./cmd/keryx','-run','TestKeryxRiverNameOnce','-count=1'],'TestKeryxRiverNameOnce')]
for name,file,old,new,cmd,expected in cases:
 p=Path(file);original=p.read_bytes();s=original.decode();assert s.count(old)==1,name
 try:
  p.write_text(s.replace(old,new));r=subprocess.run(cmd,capture_output=True);log=r.stdout+r.stderr
  (out/f'mutation-{name}.log').write_bytes(b'\n'.join(l.rstrip() for l in log.splitlines())+b'\n');assert r.returncode and expected.encode() in log,(name,log.decode());print(name,'RED',expected)
 finally:p.write_bytes(original)
 assert p.read_bytes()==original
print('byte-identical restore')
