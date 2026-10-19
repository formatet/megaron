-- Migration 192: incense (rokelse) -- a rare religious good found only on marked hexes
-- (megaron_rokelse_plan_20261019.md, order 32, Timothy 2026-10-19).
--
-- One good, `incense`. A deposit flag on map_tiles/provinces (like silver_deposit); the
-- flag sits on scrub_maquis / semi_desert hexes. Two gubbar work the hex, no building
-- (economy/hex_rules.go). Nothing requires it: no recipe, no temple required_goods.
-- Existing worlds get their flags from cmd/seed-incense, not from this migration
-- (the spacing rule is code, not SQL).
ALTER TABLE map_tiles ADD COLUMN IF NOT EXISTS incense_deposit BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE provinces ADD COLUMN IF NOT EXISTS incense_deposit BOOLEAN NOT NULL DEFAULT false;

-- labor_value 4 = worker ticks per unit (1 gubbe at 0.25/tick). Without labor_value the
-- gods would not price it (the divine valuation reads labor_value IS NOT NULL AND
-- status='active'), and an offering of it would be deducted for nothing.
-- religious = true: the temple takes its tithe when it is sold, as with wine and oil.
INSERT INTO goods (key, name, tier, category, base_value, weight, religious, status, labor_value)
VALUES ('incense', 'Incense', 'commodity', 'prestige', 1.0, 3.0, true, 'active', 4)
ON CONFLICT (key) DO NOTHING;

-- Terrain-gated on purpose: a row with NEITHER terrain nor building is the unconditional
-- trickle (economy.UnconditionalPotential) and would give every city incense.
INSERT INTO production_rules (terrain_type, building_type, good_key, rate_per_tick, requires_deposit)
SELECT t, NULL, 'incense', 0.25, 'incense'
  FROM unnest(ARRAY['scrub_maquis','semi_desert']) AS t
 WHERE NOT EXISTS (SELECT 1 FROM production_rules WHERE good_key = 'incense' AND terrain_type = t);
