-- Timothy 2026-10-28: a ship in harbour stands on its city's hex; the adjacent
-- sea hex is only where it puts out and lands. Docking used to leave the ship
-- on that sea hex, so economy.ShipDeparturePort refused it ever after.
-- Move existing harbour ships (and their embarked cohorts) into the port.
WITH misplaced AS MATERIALIZED (
 SELECT u.id,u.world_id,u.q AS previous_q,u.r AS previous_r,p.map_q,p.map_r,u.settlement_id
 FROM units u JOIN settlements s ON s.id=u.settlement_id
 JOIN provinces p ON p.id=s.province_id
 WHERE u.category='naval' AND u.status='garrison'
 AND u.q IS NOT NULL AND u.r IS NOT NULL
 AND (u.q,u.r) IS DISTINCT FROM (p.map_q,p.map_r)
 FOR UPDATE OF u
), cohorts AS (
 UPDATE units c SET q=m.map_q,r=m.map_r,updated_at=now()
 FROM misplaced m WHERE c.carried_by_ship_id=m.id AND c.status='embarked'
 RETURNING c.id
), corrected AS (
 UPDATE units u SET q=m.map_q,r=m.map_r,updated_at=now()
 FROM misplaced m WHERE u.id=m.id
 RETURNING u.id,u.world_id,m.previous_q,m.previous_r,u.q,u.r,m.settlement_id
)
INSERT INTO events(stream_id,stream_type,event_type,payload,world_id,world_tick)
SELECT c.id,'unit','ShipHarbourPositionCorrectedV1',
 jsonb_build_object('unit_id',c.id,'q',c.q,'r',c.r,'previous_q',c.previous_q,'previous_r',c.previous_r,'settlement_id',c.settlement_id,'reason','harbour_ship_on_city_hex'),
 c.world_id,w.current_tick
FROM corrected c JOIN worlds w ON w.id=c.world_id;
