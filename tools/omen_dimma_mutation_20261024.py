#!/usr/bin/env python3
"""Physical D1/D2 mutants; each arm uses a fresh DB and restores exact bytes."""
from pathlib import Path
import subprocess
ROOT=Path(__file__).resolve().parents[1]
OUT=ROOT/'docs/reviews/omen-dimma'
def arm(name,path,change,command,reason):
 p=ROOT/path;before=p.read_bytes()
 try:
  altered=change(before.decode());assert altered!=before.decode();p.write_text(altered)
  with (OUT/(name+'-red.log')).open('w') as log:r=subprocess.run(command,cwd=ROOT,stdout=log,stderr=subprocess.STDOUT)
  assert r.returncode!=0,name+' survived'
  assert reason in (OUT/(name+'-red.log')).read_text(),name+' failed for a different reason'
 finally:p.write_bytes(before)
 assert p.read_bytes()==before
arm('replacement-fog','web/static/js/megaron/render/omen.js',lambda s:s.replace("    if (phase === 'live')", "    if (phase === 'hidden') out.push({ q: p.q, r: p.r, fill: '#1C1C1C', alpha: 1 });\n    else if (phase === 'live')",1),['node','--test','web/static/js/megaron/render/omen.test.mjs'],'not ok 2 - overlay never paints hidden hexes')
arm('known-tiles','web/static/js/megaron/render/omen.js',lambda s:s.replace(".filter(t => !old.has(key(t)) || old.get(key(t)).terrain === 'fog')",''),['node','--test','web/static/js/megaron/render/omen.test.mjs'],'not ok 3 - ordinary map fog')
arm('green-spill','web/static/js/megaron/render/omen.js',lambda s:s.replace('if (regionAt) {','if (false && regionAt) {'),['node','--test','web/static/js/megaron/render/omen.test.mjs'],'not ok 5 - green follows warped pixel ownership')
arm('fishers-sent-home','server/internal/economy/recompute.go',lambda s:s.replace('if good == GoodFish && stormy[opt.Coord] {','''if good == GoodFish && stormy[opt.Coord] {
                if _, err := tx.Exec(ctx, `DELETE FROM settlement_placement WHERE settlement_id=$1 AND hex_q=$2 AND hex_r=$3 AND good_key=$4`, settlementID,opt.Coord.Q,opt.Coord.R,GoodFish);err!=nil {return err}''',1),[str(ROOT/'tools/gotest.sh'),'./internal/economy/','-run','^TestRecompute_StormOnTheHexStopsItsFishing$'],'storm removed or changed fisher placements')
with (OUT/'mutation-restored-green.log').open('w') as log:
 subprocess.run(['node','--test','web/static/js/megaron/render/omen.test.mjs'],cwd=ROOT,stdout=log,stderr=subprocess.STDOUT,check=True)
 subprocess.run([str(ROOT/'tools/gotest.sh'),'./internal/economy/','-run','^TestRecompute_StormOnTheHexStopsItsFishing$'],cwd=ROOT,stdout=log,stderr=subprocess.STDOUT,check=True)
print('PASS: replacement fog, known-tile reveal, green spill and sending fishers home -> RED; exact restores -> GREEN')
