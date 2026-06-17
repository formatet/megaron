-- Rollback 050: återställ coast_beach terräng.
UPDATE production_rules
SET terrain_type = 'coast_beach', requires_coastal = FALSE
WHERE requires_coastal = TRUE;
ALTER TABLE production_rules DROP COLUMN IF EXISTS requires_coastal;

UPDATE provinces p SET coastal = FALSE WHERE coastal = TRUE;
ALTER TABLE provinces DROP COLUMN IF EXISTS coastal;

UPDATE map_tiles SET terrain = 'coast_beach' WHERE coastal = TRUE;
ALTER TABLE map_tiles DROP COLUMN IF EXISTS coastal;
