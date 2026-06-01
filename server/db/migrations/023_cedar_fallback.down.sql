DELETE FROM production_rules
WHERE good_key = 'cedar' AND building_type = 'lumbermill' AND terrain_type IS NULL;
