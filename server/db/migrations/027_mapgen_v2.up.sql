-- Migration 027: mapgen v2
-- Adds cedar_deposit and river_valley terrain support.
-- Cedar becomes geographically scarce (Levant-region only) instead of universal lumbermill output.
-- River valley is a new fertile terrain type.

-- Cedar deposit flag on map_tiles and provinces
ALTER TABLE map_tiles  ADD COLUMN IF NOT EXISTS cedar_deposit BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE provinces  ADD COLUMN IF NOT EXISTS cedar_deposit BOOLEAN NOT NULL DEFAULT false;

-- Make lumbermill cedar production require cedar_deposit
UPDATE production_rules
SET requires_deposit = 'cedar'
WHERE good_key = 'cedar'
  AND building_type = 'lumbermill'
  AND (requires_deposit IS NULL OR requires_deposit = '');

-- River valley terrain: very fertile grain production
-- (river_valley is just a terrain string — no schema change needed)
INSERT INTO production_rules (terrain_type, building_type, good_key, rate_per_min)
VALUES
  ('river_valley', NULL,    'grain', 0.05),   -- base: 72/day (plains = 0.02/day)
  ('river_valley', 'farm',  'grain', 0.15);   -- with farm: 216/day (plains+farm ≈ 115/day)
