DELETE FROM production_rules
WHERE terrain_type = 'hills' AND building_type IS NULL AND good_key = 'grain';
