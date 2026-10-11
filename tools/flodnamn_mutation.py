#!/usr/bin/env python3
"""One real production mutation per naming, FOW, CLI, panel and label gate."""
from pathlib import Path
import subprocess
out=Path('docs/reviews/flodnamn');out.mkdir(parents=True,exist_ok=True)
cases=[
 ('world-salt','server/internal/world/river_names.go','worldID, anchor.q, anchor.r','"constant-world", anchor.q, anchor.r',['go','test','./internal/world','-run','TestRiverNamesDeterministic','-count=1'],'server'),
 ('unique','server/internal/world/river_names.go','if !used[candidate] {','if true {',['go','test','./internal/world','-run','TestRiverNamesUnique','-count=1'],'server'),
 ('delta','server/internal/world/river_names.go','if deltas[n] {','if false {',['go','test','./internal/world','-run','TestRiverNamesUnique','-count=1'],'server'),
 ('fow','server/api/handlers/world.go','if tiles[i].Visible {','if true {',['tools/gotest.sh','./api/handlers/','-run','TestMapRiverNamesFOW'],'.'),
 ('keryx','server/cmd/keryx/river_names.go','return "River " + name','return terrain',['go','test','./cmd/keryx','-run','TestKeryxRiverNamesMap','-count=1'],'server'),
 ('panel','web/static/js/megaron/render/map.js',"`${namedRiverTerrain(tile, terrainLabel(tile.terrain))} (${h.q},${h.r})`", "`${terrainLabel(tile.terrain)} (${h.q},${h.r})`",['node','--test','web/static/js/megaron/render/inspect_simple.test.mjs'],'.'),
 ('labels','web/static/js/megaron/render/river_names.js','if (zoom < .55) return [];','if (zoom < 2) return [];',['node','--test','web/static/js/megaron/render/river_names.test.mjs'],'.'),
]
for name,file,old,new,cmd,cwd in cases:
 p=Path(file);original=p.read_bytes();s=original.decode();assert old in s,(name,old)
 try:
  p.write_text(s.replace(old,new))
  r=subprocess.run(cmd,cwd=cwd,capture_output=True)
  log=r.stdout+r.stderr;(out/f'mutation-{name}.log').write_bytes(b'\n'.join(l.rstrip() for l in log.splitlines())+b'\n')
  assert r.returncode!=0 and (b'FAIL' in log or b'not ok' in log),(name,log.decode())
  print(name,'RED',flush=True)
 finally:p.write_bytes(original)
 assert p.read_bytes()==original
print('all production files restored byte-identically',flush=True)
