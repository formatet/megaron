DELETE FROM production_rules
 WHERE good_key IN ('cedar', 'timber')
   AND terrain_type = 'forest_cedar';

INSERT INTO production_rules (terrain_type, building_type, good_key, rate_per_tick, requires_deposit) VALUES
    ('forest_olive_grove', 'lumbermill', 'cedar', 6,   'cedar'),
    ('forest_olive_grove', NULL,         'cedar', 1.2, NULL),
    (NULL,                 'lumbermill', 'cedar', 3,   'cedar');
