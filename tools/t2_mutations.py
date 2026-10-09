#!/usr/bin/env python3
"""Real T2 mutations, one at a time, fresh DB, sources always restored."""
import argparse
from pathlib import Path
import re
import subprocess

ROOT=Path(__file__).resolve().parents[1]
p=argparse.ArgumentParser(description=__doc__)
p.add_argument('--output-dir',type=Path,default=ROOT/'docs/reviews/t2-lopare/mutations')
a=p.parse_args();a.output_dir.mkdir(parents=True,exist_ok=True)

def run(name,package,pattern,red=False):
    result=subprocess.run(['tools/gotest.sh',package,'-run',pattern,'-timeout','45s'],cwd=ROOT,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True)
    (a.output_dir/(name+'.log')).write_text(result.stdout)
    failures=re.findall(r'--- FAIL: (\S+)',result.stdout)
    if red:
        assert result.returncode and failures and 'build failed' not in result.stdout and 'panic: test timed out' not in result.stdout,(name,result.stdout[-1200:])
        assert any(re.search(pattern,f) for f in failures),(name,failures)
        print(name+': RED ('+', '.join(failures)+')',flush=True)
    else:
        assert result.returncode==0,(name,result.stdout[-1200:])
        print(name+': GREEN',flush=True)

cases=[
 ('dispatch-without-contents','server/internal/messenger/carrier_fate.go','./internal/messenger/','TestCarrierFate_StormTransportLosesPassenger',
  lambda s:s.replace('lossNotice = LossNotice{w.MessengerID, w.SenderID, w.Reason, w.Tick, w.Envelope}','lossNotice = LossNotice{w.MessengerID, w.SenderID, w.Reason, w.Tick, nil}',1)),
 ('rescue-without-next-port','server/internal/messenger/carrier_fate.go','./internal/messenger/','TestCarrierFate_PendingRescueCanLandBeforeProjection',
  lambda s:s.replace('if err := h.landCarrierPassenger(ctx, tx, world, currentTick, w); err != nil {','if err := error(nil); err != nil {',1)),
 # Deliberately duplicate the archived row on a replayed scan, without relying
 # on a changed RNG or another voyage. The second scan test must reject it.
 ('double-scan-double-dispatch','server/internal/messenger/passage.go','./internal/messenger/','TestCarrierFate_StormTransportLosesPassenger',
  lambda s:s.replace('if err := h.consumeCarrierWitnesses(ctx, e.WorldID, currentTick); err != nil {',
  '''if _,err:=h.pool.Exec(ctx,`INSERT INTO notifications(world_id,player_id,kind,level,body_json) SELECT world_id,player_id,kind,level,body_json FROM notifications WHERE world_id=$1 AND kind='MessengerLostAtSea'`,e.WorldID);err!=nil{return err}
 if err := h.consumeCarrierWitnesses(ctx, e.WorldID, currentTick); err != nil {''',1)),
 ('storm-as-battle','server/internal/combat/sea_storm.go','./internal/messenger/','TestCarrierFate_StormMissionLosesPassenger',
  lambda s:s.replace('worldID, v.shipID, nil, "storm"','worldID, v.shipID, &v.shipID, "storm"',1)),
 ('leak-carrier-to-sender','server/api/handlers/messenger.go','./api/handlers/','TestRescuedRunner_SenderOutboxMapAndEyesKeepPositionUnknown',
  lambda s:s.replace('CASE WHEN secret.no_word THEN NULL ELSE m.carrier_name END','m.carrier_name')),
 ('upkeep-port-as-loss','server/internal/combat/upkeep_carrier_fate.go','./internal/combat/','TestUpkeepCarrierFate_HarbourAndSea',
  lambda s:s.replace('if settlement != nil {','if settlement != nil && false {',1)),
 ('upkeep-sea-as-home','server/internal/combat/upkeep_carrier_fate.go','./internal/combat/','TestUpkeepCarrierFate_HarbourAndSea',
  lambda s:s.replace('settlement, q, r, err := h.carrierDeathLocation(ctx, tx, world, u.id, state, currentTick)', 'state.status = "garrison"; state.settlement = &u.ownerID\n if err := tx.QueryRow(ctx, `SELECT origin_id FROM messengers WHERE world_id=$1 LIMIT 1`, world).Scan(&state.settlement); err != nil { return err }\n settlement, q, r, err := h.carrierDeathLocation(ctx, tx, world, u.id, state, currentTick)',1)),
 ('stale-timer-before-projection','server/internal/messenger/handler.go','./internal/messenger/','TestCarrierFate_StormTransportLosesPassenger',
  lambda s:s.replace('if pending || status == "lost" {','if status == "lost" {',1).replace('pending, err := pendingCarrierWitness(ctx, tx, payload.MessengerID)','_, err = pendingCarrierWitness(ctx, tx, payload.MessengerID)',1)),
]
run('baseline-messenger','./internal/messenger/','TestCarrierFate_')
run('baseline-upkeep','./internal/combat/','TestUpkeepCarrierFate_')
run('baseline-visibility','./api/handlers/','TestRescuedRunner_')
for name,file,package,pattern,change in cases:
    path=ROOT/file;original=path.read_text();mutated=change(original)
    assert mutated!=original,name+': mutation did not change source'
    try:
        path.write_text(mutated);subprocess.run(['gofmt','-w',str(path)],check=True)
        run(name,package,pattern,True)
    finally:path.write_text(original)
run('restored-messenger','./internal/messenger/','TestCarrierFate_')
run('restored-upkeep','./internal/combat/','TestUpkeepCarrierFate_')
run('restored-visibility','./api/handlers/','TestRescuedRunner_')
print(f'All {len(cases)} real mutations rejected; sources restored.',flush=True)
