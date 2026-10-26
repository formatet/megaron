-- A return-leg redirect could keep explore_return after canceling its mission.
-- The legacy arrival then attached a land unit to its old home while its actual
-- arrival coordinates remained elsewhere. Correct the relation, never teleport.
-- Naval harbour garrisons deliberately occupy an adjacent sea hex and are exempt.
WITH misplaced AS MATERIALIZED (
 SELECT u.id,u.world_id,u.owner_id,u.q,u.r,u.settlement_id AS previous_settlement_id
 FROM units u JOIN settlements s ON s.id=u.settlement_id
 JOIN provinces p ON p.id=s.province_id
 WHERE u.category='land' AND u.status='garrison'
 AND u.q IS NOT NULL AND u.r IS NOT NULL
 AND (u.q,u.r) IS DISTINCT FROM (p.map_q,p.map_r)
 FOR UPDATE OF u
), corrected AS (
 UPDATE units u SET status='positioned',settlement_id=NULL,updated_at=now()
 FROM misplaced m WHERE u.id=m.id
 RETURNING u.id,u.world_id,m.previous_settlement_id,u.q,u.r
)
INSERT INTO events(stream_id,stream_type,event_type,payload,world_id,world_tick)
SELECT c.id,'unit','UnitGarrisonLocationCorrectedV1',
 jsonb_build_object('unit_id',c.id,'q',c.q,'r',c.r,'previous_settlement_id',c.previous_settlement_id,'status','positioned','reason','land_garrison_away_from_city'),
 c.world_id,w.current_tick
FROM corrected c JOIN worlds w ON w.id=c.world_id;
