BEGIN READ ONLY;
-- world
SELECT id,name,current_tick,last_tick_at,map_seed,map_width,map_height,state,status,now() AS observed_at FROM worlds WHERE id='2178e795-1303-4cc3-865a-6386847d68d1';
-- tiles
SELECT q,r,terrain,coastal,landmass_id FROM map_tiles WHERE world_id='2178e795-1303-4cc3-865a-6386847d68d1' ORDER BY q,r;
-- pylos
SELECT s.id,s.name,p.wanax_name,v.map_q AS q,v.map_r AS r FROM settlements s JOIN players p ON p.id=s.owner_id JOIN provinces v ON v.id=s.province_id WHERE s.world_id='2178e795-1303-4cc3-865a-6386847d68d1' AND (s.name ILIKE 'Pylos' OR p.wanax_name ILIKE 'Talthybios');
-- naval
SELECT u.id,u.name,u.type,u.status,u.size,u.crew,u.q,u.r,u.provisions,u.provision_goods,u.settlement_id,p.wanax_name FROM units u JOIN players p ON p.id=u.owner_id WHERE u.world_id='2178e795-1303-4cc3-865a-6386847d68d1' AND u.category='naval' ORDER BY p.wanax_name,u.id;
-- transports
SELECT id,kind,status,ship_unit_id,origin_id,dest_id,origin_q,origin_r,dest_q,dest_r,departed_tick,due_tick,journey FROM transports WHERE world_id='2178e795-1303-4cc3-865a-6386847d68d1' AND category='naval' ORDER BY created_at DESC LIMIT 30;
-- schema
SELECT version,dirty FROM schema_migrations;
COMMIT;

BEGIN READ ONLY;
-- identity
SELECT s.id,s.name,p.username,p.wanax_name,v.map_q,v.map_r FROM settlements s JOIN players p ON p.id=s.owner_id JOIN provinces v ON v.id=s.province_id WHERE s.id='47d92957-6871-43e8-9a7e-91d934f7cf5c';
-- upkeep_queue
SELECT event_type,due_tick,processed_at,failed_at FROM scheduled_events WHERE world_id='2178e795-1303-4cc3-865a-6386847d68d1' AND event_type='UpkeepTick' ORDER BY due_tick DESC LIMIT 5;
COMMIT;

BEGIN READ ONLY;
-- Independent recursive SQL cross-check
WITH RECURSIVE water AS (
 SELECT q,r FROM map_tiles WHERE world_id='2178e795-1303-4cc3-865a-6386847d68d1'
 AND terrain IN ('coastal_sea','deep_sea','river','river_ford')
), reach(q,r) AS (
 SELECT q,r FROM water WHERE q=39 AND r=28
 UNION
 SELECT w.q,w.r FROM reach a CROSS JOIN (VALUES(1,0),(1,-1),(0,-1),(-1,0),(-1,1),(0,1)) d(dq,dr)
 JOIN water w ON w.q=a.q+d.dq AND w.r=a.r+d.dr
)
SELECT (SELECT count(*) FROM reach) AS component_size,
 (SELECT jsonb_agg(jsonb_build_array(q,r) ORDER BY q,r) FROM reach) AS hexes,
 (SELECT min(greatest(abs(a.q-w.q),abs(a.r-w.r),abs(a.q+a.r-w.q-w.r)))
 FROM reach a CROSS JOIN water w WHERE NOT EXISTS (SELECT 1 FROM reach b WHERE b.q=w.q AND b.r=w.r)) AS nearest_other_distance;
COMMIT;
