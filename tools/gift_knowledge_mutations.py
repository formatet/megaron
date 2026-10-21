#!/usr/bin/env python3
"""Restore each obsolete gift visibility gate and require a named failure."""
from pathlib import Path
import subprocess
import sys
ROOT = Path(__file__).resolve().parents[1]
OUT = Path(sys.argv[1]).resolve()
OUT.mkdir(parents=True, exist_ok=True)
for name,filename,old,new in [
 ('list','server/api/handlers/gift_destinations.go',
  'knownToPlayer(eyes, remembered, province.MapPosition{Q: q, R: r}, terrain)',
  'province.VisibleFrom(province.MapPosition{Q: q, R: r}, legacyOrigins, 6)'),
 ('send','server/api/handlers/province.go',
  'knownToPlayer(loadLiveEyes(r.Context(), h.pool, worldID, playerID, h.clk.Now()), loadRememberedTiles(r.Context(), h.pool, worldID, playerID), province.MapPosition{Q: destQ, R: destR}, destTerrain)',
  'province.VisibleFrom(province.MapPosition{Q: destQ, R: destR}, loadVisibleOrigins(r.Context(), h.pool, worldID, playerID), 6)')]:
 p=ROOT/filename;original=p.read_text();assert original.count(old)==1
 try:
  mutated=original.replace(old,new,1)
  if name=='list':
   mutated=mutated.replace('eyes := loadLiveEyes(r.Context(), h.pool, worldID, player, h.clk.Now())','legacyOrigins := loadVisibleOrigins(r.Context(), h.pool, worldID, player)').replace('remembered := loadRememberedTiles(r.Context(), h.pool, worldID, player)','_ = loadRememberedTiles(r.Context(), h.pool, worldID, player)')
  p.write_text(mutated)
  result=subprocess.run(['tools/gotest.sh','./api/handlers','-run','TestGiftOldRadiusHaloDoesNotRevealOrPermitDestination'],cwd=ROOT,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
  (OUT/(name+'.log')).write_text(result.stdout)
  assert result.returncode and '--- FAIL: TestGiftOldRadiusHaloDoesNotRevealOrPermitDestination' in result.stdout,result.stdout
  print(name+': named RED',flush=True)
 finally:p.write_text(original)
