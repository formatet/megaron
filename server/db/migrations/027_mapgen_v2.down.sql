ALTER TABLE map_tiles DROP COLUMN IF EXISTS cedar_deposit;
ALTER TABLE provinces DROP COLUMN IF EXISTS cedar_deposit;

UPDATE production_rules SET requires_deposit = NULL
WHERE good_key = 'cedar' AND building_type = 'lumbermill' AND requires_deposit = 'cedar';

DELETE FROM production_rules WHERE terrain_type = 'river_valley';
