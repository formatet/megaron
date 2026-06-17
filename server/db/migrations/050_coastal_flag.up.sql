-- Migration 050: "coast är ingen terräng" — ersätt coast_beach med coastal-flagga.
--
-- coast_beach var ett terräng-värde för kuststrand. Den ersätts av en booleansk
-- flagga (coastal = granne till coastal_sea), i linje med design-invarianten
-- "Coast är ingen terräng — egenskap = granne till hav".
--
-- Berörda tabeller:
--   map_tiles       → add coastal BOOL, convert coast_beach → plains
--   provinces       → add coastal BOOL, copy from map_tiles
--   production_rules → add requires_coastal BOOL, update fish-regler

-- 1. map_tiles
ALTER TABLE map_tiles ADD COLUMN IF NOT EXISTS coastal BOOLEAN NOT NULL DEFAULT FALSE;
UPDATE map_tiles SET coastal = TRUE WHERE terrain = 'coast_beach';
UPDATE map_tiles SET terrain = 'plains' WHERE terrain = 'coast_beach';

-- 2. provinces
ALTER TABLE provinces ADD COLUMN IF NOT EXISTS coastal BOOLEAN NOT NULL DEFAULT FALSE;
UPDATE provinces p
SET coastal = mt.coastal
FROM map_tiles mt
WHERE mt.world_id = p.world_id AND mt.q = p.map_q AND mt.r = p.map_r;

-- 3. production_rules
ALTER TABLE production_rules ADD COLUMN IF NOT EXISTS requires_coastal BOOLEAN NOT NULL DEFAULT FALSE;
UPDATE production_rules
SET requires_coastal = TRUE, terrain_type = NULL
WHERE terrain_type = 'coast_beach';

-- 4. Ta bort dubbletter (de två fish/coast_beach-raderna ger nu dubbla fish/coastal-regler)
DELETE FROM production_rules pr1
USING production_rules pr2
WHERE pr1.id > pr2.id
  AND pr1.good_key = pr2.good_key
  AND (pr1.terrain_type IS NOT DISTINCT FROM pr2.terrain_type)
  AND (pr1.building_type IS NOT DISTINCT FROM pr2.building_type)
  AND pr1.requires_coastal = pr2.requires_coastal;
