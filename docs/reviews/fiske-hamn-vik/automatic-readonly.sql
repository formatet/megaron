\pset pager off
BEGIN READ ONLY;
-- Conservative unaffected-food lower bound: building-free grain/fish terrain rows only.
SELECT s.name,s.population%100 AS remainder_people,MAX(pr.rate_per_tick) AS unaffected_base_food_lower_bound
FROM settlements s JOIN worlds w ON w.id=s.world_id JOIN provinces p ON p.id=s.province_id
JOIN map_tiles mt ON mt.world_id=s.world_id AND mt.terrain<>'deep_sea'
AND greatest(abs(mt.q-p.map_q),abs(mt.r-p.map_r),abs(mt.q-p.map_q+mt.r-p.map_r)) BETWEEN 1 AND 2
JOIN production_rules pr ON pr.terrain_type=mt.terrain AND pr.building_type IS NULL AND pr.good_key IN ('grain','fish')
AND pr.requires_deposit IS NULL AND (NOT pr.requires_coastal OR mt.coastal)
WHERE w.status='active' AND s.state NOT IN ('collapsed','sunk')
GROUP BY s.id,s.name,s.population ORDER BY s.name;
SELECT mt.world_id,count(*) AS coastal_hexes_with_four_land_neighbors FROM map_tiles mt JOIN worlds w ON w.id=mt.world_id
WHERE w.status='active' AND mt.terrain='coastal_sea' AND
(SELECT count(*) FROM map_tiles n WHERE n.world_id=mt.world_id
AND (n.q-mt.q,n.r-mt.r) IN ((1,0),(-1,0),(0,1),(0,-1),(1,-1),(-1,1))
AND n.terrain NOT IN ('sea','coastal_sea','deep_sea'))>=4 GROUP BY mt.world_id;
COMMIT;
