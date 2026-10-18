#!/usr/bin/env python3
"""Physical compiling mutations; named assertion RED, exact restoration, fresh GREEN."""
from pathlib import Path
import hashlib,os,socket,subprocess,sys
ROOT=Path(__file__).resolve().parents[3];OUT=Path(__file__).resolve().parent
api=lambda name:['tools/gotest.sh','./api/handlers','-run',name,'-v']
econ=lambda name:['tools/gotest.sh','./internal/economy','-run',name,'-v']
cases=[
 ('post-lock', 'server/api/handlers/settlement_placement.go', '// Settlement first, then hex, then placements/goods. Recheck ownership and\n\t// population after waiting: the pre-TX display lookup may now be stale.\n\tif err := tx.QueryRow(r.Context(), `SELECT population FROM settlements WHERE id=$1 AND owner_id=$2 AND world_id=$3 FOR UPDATE`', '// Settlement first, then hex, then placements/goods. Recheck ownership and\n\t// population after waiting: the pre-TX display lookup may now be stale.\n\tif err := tx.QueryRow(r.Context(), `SELECT population FROM settlements WHERE id=$1 AND owner_id=$2 AND world_id=$3`', api('TestPlacementConcurrentPOSTsSerialize'), 'TestPlacementConcurrentPOSTsSerialize'),
 ('delete-lock', 'server/api/handlers/settlement_placement.go', 'var population int\n\tif err := tx.QueryRow(r.Context(), `SELECT population FROM settlements WHERE id=$1 AND owner_id=$2 AND world_id=$3 FOR UPDATE`', 'var population int\n\tif err := tx.QueryRow(r.Context(), `SELECT population FROM settlements WHERE id=$1 AND owner_id=$2 AND world_id=$3`', api('TestPlacementDELETEWaitsBeforeLockingPlacement'), 'TestPlacementDELETEWaitsBeforeLockingPlacement'),
 ('growth-lock', 'server/internal/economy/placement_lock.go', 'WHERE id=$1 FOR UPDATE`', 'WHERE id=$1`', econ('TestPlacementGrowthConcurrentRespectsCapacity'), 'TestPlacementGrowthConcurrentRespectsCapacity'),
 ('growth-ordinal', 'server/internal/economy/founding_placement.go', 'gubbeOrdinal < 1 || gubbeOrdinal > population/100', 'gubbeOrdinal < 1', econ('TestPlacementGrowthRejectsStaleOrdinalAndFullPool'), 'TestPlacementGrowthRejectsStaleOrdinalAndFullPool'),
 ('growth-pool', 'server/internal/economy/founding_placement.go', 'placedCounts.Total >= population/100', 'false', econ('TestPlacementGrowthRejectsStaleOrdinalAndFullPool'), 'TestPlacementGrowthRejectsStaleOrdinalAndFullPool'),
 ('take-no-wait', 'server/api/handlers/settlement_placement.go', 'WHERE id=$1 FOR UPDATE NOWAIT`', 'WHERE id=$1 FOR UPDATE`', api('TestPlacementTakeBusyHolderReturnsConflict'), 'TestPlacementTakeBusyHolderReturnsConflict'),
]
cases += [
 ('terrain-pool','server/api/handlers/settlement_placement.go','if err := tx.QueryRow(r.Context(),\n\t\t\t`SELECT terrain FROM map_tiles','if err := h.pool.QueryRow(r.Context(),\n\t\t\t`SELECT terrain FROM map_tiles',api('TestPlacementSingleConnectionCannotStarve'),'TestPlacementSingleConnectionCannotStarve'),
 ('eyes-pool','server/api/handlers/settlement_placement.go','loadLiveEyes(r.Context(), tx,','loadLiveEyes(r.Context(), h.pool,',api('TestPlacementSingleConnectionCannotStarve'),'TestPlacementSingleConnectionCannotStarve'),
 ('memory-pool','server/api/handlers/settlement_placement.go','loadRememberedTiles(r.Context(), tx,','loadRememberedTiles(r.Context(), h.pool,',api('TestPlacementSingleConnectionCannotStarve'),'TestPlacementSingleConnectionCannotStarve'),
]
cases += [
 ('slaughter-order','server/api/handlers/settlement_placement.go','SELECT population FROM settlements WHERE id = $1 AND owner_id=$2 AND world_id=$3 FOR UPDATE`','SELECT population FROM settlements WHERE id = $1 AND owner_id=$2 AND world_id=$3`',api('TestPlacementSlaughterLocksSettlementBeforeGoods'),'TestPlacementSlaughterLocksSettlementBeforeGoods'),
]
cases=cases[int(sys.argv[1]):] if len(sys.argv)>1 else cases
results=[]
for name,relative,old,new,cmd,named in cases:
 p=ROOT/relative;original=p.read_bytes();source=original.decode();assert source.count(old)==1,(name,source.count(old))
 def run(label,green):
  env={k:os.environ[k] for k in ('HOME','PATH')}
  if cmd[0]=='tools/gotest.sh':
   with socket.socket() as sock:sock.bind(('127.0.0.1',0));env['GOTEST_PORT']=str(sock.getsockname()[1])
  r=subprocess.run(cmd,cwd=ROOT,env=env,capture_output=True,text=True);log=r.stdout+r.stderr;(OUT/(name+'-'+label+'.log')).write_text(log)
  if green:assert r.returncode==0,(name,log[-3000:])
  else:assert r.returncode and named in log and ('--- FAIL:' in log or 'not ok' in log) and '[build failed]' not in log,(name,log[-3000:])
 try:p.write_text(source.replace(old,new));run('red',False)
 finally:p.write_bytes(original)
 run('restored',True);assert p.read_bytes()==original
 print(name,'named assertion RED -> exact bytes -> GREEN',hashlib.sha256(original).hexdigest(),flush=True)
