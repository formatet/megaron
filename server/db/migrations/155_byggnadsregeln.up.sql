-- Byggnadsregeln (megaron_plan_byggnadsregeln.md): a hex-bound building gives
-- its hex +4 worker places and multiplies the hex's OWN yield per worker by
-- (1 + 0.7 * level). The rule lives in Go (economy/hex_rules.go); production_rules
-- therefore carries only the BASE rate per worker (r0) for terrain/deposit rows.

-- 1. Terrain rows become "base rate per worker" everywhere. They used to be
--    "per hex / capL1"; where capL1 was the fallback (2) the row is halved.
UPDATE production_rules SET rate_per_tick = rate_per_tick / 2
 WHERE building_type IS NULL AND terrain_type IS NOT NULL
   AND ( (good_key = 'stone' AND terrain_type IN ('hills','mountain_limestone'))
      OR (good_key = 'oil'   AND terrain_type IN ('forest_olive_grove','hills','plains'))
      OR (good_key = 'wine'  AND terrain_type IN ('hills','plains','river_valley','scrub_maquis'))
      OR (good_key = 'timber' AND terrain_type = 'forest_cedar') );

-- Olive press / winery terrain rows (P6 boost potential) were divided by the same fallback cap.
UPDATE production_rules SET rate_per_tick = rate_per_tick / 2
 WHERE terrain_type IS NOT NULL
   AND ( (building_type = 'olive_press' AND good_key = 'oil')
      OR (building_type = 'winery'      AND good_key = 'wine') );

-- 2. Silver: keep the two mine rows, now as base rate per worker that preserves
--    today's level-1 figure (rate / 5 places / 1.7).
UPDATE production_rules SET rate_per_tick = 11.52 / 5.0 / 1.7
 WHERE building_type = 'mine' AND good_key = 'silver' AND terrain_type = 'hills';
UPDATE production_rules SET rate_per_tick = 28.8 / 5.0 / 1.7
 WHERE building_type = 'mine' AND good_key = 'silver' AND terrain_type = 'mountain_limestone';

-- 3. The building rows are replaced by the rule.
DELETE FROM production_rules
 WHERE building_type IN ('farm','harbour','lumbermill','stonequarry')
    OR (building_type = 'mine' AND good_key <> 'silver');
