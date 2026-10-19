-- Timothy/Claude 2026-10-19: a little timber on maquis, hills and river valley.
-- This is the per-worker base rate. Places and the lumbermill link live in
-- economy.hexRules; no stock, placement or existing forest rule is changed.
INSERT INTO production_rules (terrain_type, building_type, good_key, rate_per_tick, requires_coastal)
SELECT terrain, NULL, 'timber', 0.33, false
FROM unnest(ARRAY['scrub_maquis','hills','river_valley']) AS terrain
WHERE NOT EXISTS (
    SELECT 1 FROM production_rules existing
    WHERE existing.terrain_type = terrain
      AND existing.building_type IS NULL AND existing.good_key = 'timber'
);
