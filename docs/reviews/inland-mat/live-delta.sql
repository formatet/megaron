-- READ ONLY, evaluated on live 2026-10-17. Fixed-state marginal effect of
-- raising hills grain r0 by 1/6, including the population-remainder term.
-- No city names, player identities, inventories or hidden map exported.
WITH h AS (
 SELECT s.id,s.population,mt.q,mt.r,coalesce(b.level,0) AS level
 FROM settlements s JOIN provinces p ON p.id=s.province_id
 JOIN map_tiles mt ON mt.world_id=s.world_id AND mt.terrain='hills'
 AND greatest(abs(mt.q-p.map_q),abs(mt.r-p.map_r),abs(mt.q-p.map_q+mt.r-p.map_r)) BETWEEN 1 AND 2
 LEFT JOIN buildings b ON b.settlement_id=s.id AND b.hex_q=mt.q AND b.hex_r=mt.r AND b.building_type='farm'
 WHERE s.owner_id IS NOT NULL AND s.state NOT IN ('sunk','collapsed')
), delta AS (
 SELECT h.id,
 SUM((0.5-1.0/3.0)*(1+0.7*h.level)*(least((SELECT count(*) FROM settlement_placement sp WHERE sp.settlement_id=h.id AND sp.hex_q=h.q AND sp.hex_r=h.r AND sp.good_key='grain'),CASE WHEN h.level>0 THEN 5 ELSE 1 END) + (h.population%100)/100.0)) AS extra_grain_rate
 FROM h GROUP BY h.id
)
SELECT 'hills-bearing owned settlements',count(*),'negative deltas',count(*) FILTER(WHERE extra_grain_rate<0),'positive deltas',count(*) FILTER(WHERE extra_grain_rate>0),'total added grain rate',sum(extra_grain_rate) FROM delta;
