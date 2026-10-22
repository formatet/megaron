\pset pager off
BEGIN READ ONLY;
SELECT clock_timestamp() AS observed_at, version, dirty FROM schema_migrations;
SELECT terrain_type,good_key,rate_per_tick,building_type FROM production_rules WHERE good_key='fish' ORDER BY terrain_type;
SELECT s.name,w.id AS world_id,w.status AS world_status,w.current_tick,sp.hex_q,sp.hex_r,
 count(*) AS workers,coalesce(max(b.level),0) AS harbour_level,
 count(*)* (SELECT sum(rate_per_tick) FROM production_rules WHERE terrain_type='deep_sea' AND good_key='fish' AND building_type IS NULL) AS old_fish_per_tick,
 CASE WHEN coalesce(max(b.level),0)=0 THEN 0 ELSE least(count(*),4)*(1+0.7*max(b.level))*
 (SELECT sum(rate_per_tick) FROM production_rules WHERE terrain_type='deep_sea' AND good_key='fish' AND building_type IS NULL) END AS proposed_fish_per_tick
FROM settlement_placement sp JOIN settlements s ON s.id=sp.settlement_id JOIN worlds w ON w.id=s.world_id
JOIN map_tiles mt ON mt.world_id=s.world_id AND mt.q=sp.hex_q AND mt.r=sp.hex_r
LEFT JOIN buildings b ON b.settlement_id=s.id AND b.building_type='harbour'
WHERE sp.target_kind='hex' AND sp.good_key='fish' AND mt.terrain='deep_sea'
GROUP BY s.id,s.name,w.id,w.status,w.current_tick,sp.hex_q,sp.hex_r ORDER BY s.name,sp.hex_q,sp.hex_r;
SELECT w.status,count(*) AS deep_placements,count(DISTINCT s.id) AS cities FROM settlement_placement sp
JOIN settlements s ON s.id=sp.settlement_id JOIN worlds w ON w.id=s.world_id
JOIN map_tiles mt ON mt.world_id=s.world_id AND mt.q=sp.hex_q AND mt.r=sp.hex_r
WHERE sp.target_kind='hex' AND sp.good_key='fish' AND mt.terrain='deep_sea' GROUP BY w.status;
SELECT s.name,s.state,s.population,coalesce(max(b.level),0) AS harbour_level,
 count(DISTINCT mt.q::text||','||mt.r::text) AS deep_hexes_in_catchment
FROM settlements s JOIN provinces p ON p.id=s.province_id JOIN worlds w ON w.id=s.world_id
JOIN map_tiles mt ON mt.world_id=s.world_id AND mt.terrain='deep_sea'
AND greatest(abs(mt.q-p.map_q),abs(mt.r-p.map_r),abs(mt.q-p.map_q+mt.r-p.map_r)) BETWEEN 1 AND 2
LEFT JOIN buildings b ON b.settlement_id=s.id AND b.building_type='harbour'
WHERE w.status='active' AND s.state NOT IN ('collapsed','sunk')
GROUP BY s.id,s.name,s.state,s.population ORDER BY s.name;
COMMIT;
