BEGIN READ ONLY;
SELECT json_agg(x ORDER BY x.name) FROM (
 SELECT s.name, count(mt.q) FILTER (WHERE pr.rate_per_tick>0) AS timber_hexes
 FROM settlements s JOIN provinces p ON p.id=s.province_id
 LEFT JOIN map_tiles mt ON mt.world_id=p.world_id
  AND greatest(abs(mt.q-p.map_q),abs(mt.r-p.map_r),abs(mt.q-p.map_q+mt.r-p.map_r)) BETWEEN 1 AND 2
 LEFT JOIN production_rules pr ON pr.terrain_type=mt.terrain
  AND pr.good_key='timber' AND pr.building_type IS NULL
 WHERE s.state='active' GROUP BY s.id,s.name
) x;
COMMIT;
