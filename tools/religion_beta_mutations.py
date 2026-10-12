#!/usr/bin/env python3
"""Checkpoint B physical mutations: named red, byte restore, fresh green."""
from pathlib import Path
import subprocess,json,os,socket
ROOT=Path(__file__).resolve().parents[1]
OUT=ROOT/'docs/reviews/religion-alfa/mutations-b';OUT.mkdir(parents=True,exist_ok=True)
cases=[
 ('thread-flag','server/internal/transport/departure.go','if err != nil || !blessed {','if err != nil || !blessed || j.Category == "land" {','./internal/transport','TestBetaTransportFrozenDeparture'),
 ('thread-radius','server/internal/transport/departure.go','return 1\n','return 2\n','./internal/transport','TestBetaTransportFrozenDeparture'),
 ('runner-factor','server/internal/messenger/departure_prayer.go','*religion.DepartureTimeFactor','*1.0','./api/handlers','TestBetaRunnerRealSendConsumesThread'),
 ('stillness-speed','server/internal/combat/march_start.go','departureFactor = religion.DepartureTimeFactor','departureFactor = 1.0','./internal/combat','TestBetaShipDepartureSpeedLatePrayerAndActualReturn'),
 ('stillness-storm','server/internal/combat/sea_storm.go','if !hit || protected {','if !hit || (false && protected) {','./internal/combat','TestBetaStillnessDamageSnapshotAndReturn'),
 ('freight-stamp','server/internal/transport/transport.go','nullableBlessingTick(p.Journey.Stillness, p.DepartedTick)','nullableBlessingTick(false, p.DepartedTick)','./internal/transport','TestBetaFreightStillnessFrozenOnShip'),
 ('storm-owner','server/internal/combat/sea_storm.go','AND stillness_owner_id=$4 AND owner_id=$4','AND owner_id=$4','./internal/combat','TestBetaStillnessDoesNotFollowShipOwner'),
 ('standing-return','server/internal/combat/standing_orders.go','o.toID, &journey, false','o.toID, &journey, true','./internal/combat','TestBetaStandingReturnDoesNotConsumePendingGift'),
 ('return-consume','server/internal/transport/departure.go','if !outward {','if false && !outward {','./api/handlers','TestBetaDeparturePrayerCastConsumeLateAndReturn'),
 ('late-caravan','server/api/handlers/settlement.go','message = "Asterion\'s thread waits','_,err=tx.Exec(r.Context(),`UPDATE transports SET due_tick=due_tick-1 WHERE origin_id=$1 AND status=\'in_transit\'`,settlementID)\n   message = "Asterion\'s thread waits','./api/handlers','TestBetaPrayerAfterActualTransportDeparture'),
 ('late-ship','server/api/handlers/settlement.go','message = "The Sea Daimons of Stillness shelter','_,err=tx.Exec(r.Context(),`UPDATE units SET stillness_depart_tick=depart_tick,stillness_arrive_tick=arrive_tick,stillness_owner_id=owner_id WHERE world_id=$1 AND owner_id=$2 AND status=\'marching\' AND category=\'naval\'`,worldID,playerID)\n    message = "The Sea Daimons of Stillness shelter','./api/handlers','TestBetaPrayerAfterShipDepartureDoesNotProtectVoyage'),
 ('army-eligible','server/internal/combat/march_start.go','if unit.CategoryOf(u.Type) == unit.CategoryNaval && u.SettlementID != nil {','if u.SettlementID != nil {','./internal/combat','TestBetaArmyDoesNotConsumeDeparturePrayers'),
]
results=[]
for name,file,needle,replacement,pkg,test in cases:
 p=ROOT/file;original=p.read_bytes();source=original.decode();assert needle in source,(name,'missing anchor')
 try:
  p.write_text(source.replace(needle,replacement,1))
  sock=socket.socket();sock.bind(('127.0.0.1',0));port=sock.getsockname()[1];sock.close()
  env=dict(os.environ,GOTEST_PORT=str(port))
  r=subprocess.run([str(ROOT/'tools/gotest.sh'),pkg,'-run','^'+test+'$'],cwd=ROOT,env=env,text=True,capture_output=True)
  (OUT/(name+'-red.log')).write_text(r.stdout+r.stderr)
  assert r.returncode!=0 and '--- FAIL: '+test in r.stdout,(name,'missing named red',r.stdout,r.stderr)
 finally:p.write_bytes(original)
 assert p.read_bytes()==original
 results.append({'mutation':name,'test':test,'named_red':True,'byte_restored':True})
 print(name,'named red; byte restored',flush=True)
r=subprocess.run([str(ROOT/'tools/gotest.sh'),'./api/handlers','./internal/combat','./internal/transport','-run','^TestBeta'],cwd=ROOT,text=True,capture_output=True)
(OUT/'restored-fresh.log').write_text(r.stdout+r.stderr);assert r.returncode==0,(r.stdout,r.stderr)
for item in results:item['restored_fresh_green']=True
(OUT/'results.json').write_text(json.dumps(results,indent=2)+'\n')
