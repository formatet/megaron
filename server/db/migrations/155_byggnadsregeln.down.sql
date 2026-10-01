-- Reverses 155: restore the deleted building rows with their original ids/values,
-- silver to its per-hex figure, and the terrain rows to their per-hex/capL1 unit.
INSERT INTO production_rules (id, terrain_type, building_type, good_key, rate_per_tick, requires_deposit, requires_coastal) VALUES
 (8,  'plains',             'farm',        'grain',  1.6999999999999997, NULL,     false),
 (9,  'plains',             'farm',        'oil',    1.9999999999999998, NULL,     false),
 (10, 'hills',              'farm',        'wine',   3.9999999999999996, NULL,     false),
 (13, 'hills',              'mine',        'copper', 8.5,                'copper', false),
 (14, 'mountain_limestone', 'mine',        'tin',    8.5,                'tin',    false),
 (19, NULL,                 'mine',        'stone',  3.333333333333333,  NULL,     false),
 (20, NULL,                 'stonequarry', 'stone',  6.666666666666666,  NULL,     false),
 (29, 'river_valley',       'farm',        'grain',  2.833333333333333,  NULL,     false),
 (31, NULL,                 'lumbermill',  'timber', 3.4,                NULL,     false),
 (38, 'river_delta',        'farm',        'grain',  4.533333333333332,  NULL,     false),
 (42, 'coastal_sea',        'harbour',     'fish',   6.8,                NULL,     false),
 (46, 'forest_cedar',       'lumbermill',  'cedar',  6.8,                NULL,     false),
 (49, 'plains',             'farm',        'wine',   1.9999999999999998, NULL,     false),
 (54, 'river_valley',       'farm',        'wine',   1.9999999999999998, NULL,     false);

UPDATE production_rules SET rate_per_tick = 11.52
 WHERE building_type = 'mine' AND good_key = 'silver' AND terrain_type = 'hills';
UPDATE production_rules SET rate_per_tick = 28.799999999999997
 WHERE building_type = 'mine' AND good_key = 'silver' AND terrain_type = 'mountain_limestone';

UPDATE production_rules SET rate_per_tick = rate_per_tick * 2
 WHERE terrain_type IS NOT NULL
   AND ( (building_type = 'olive_press' AND good_key = 'oil')
      OR (building_type = 'winery'      AND good_key = 'wine') );

UPDATE production_rules SET rate_per_tick = rate_per_tick * 2
 WHERE building_type IS NULL AND terrain_type IS NOT NULL
   AND ( (good_key = 'stone' AND terrain_type IN ('hills','mountain_limestone'))
      OR (good_key = 'oil'   AND terrain_type IN ('forest_olive_grove','hills','plains'))
      OR (good_key = 'wine'  AND terrain_type IN ('hills','plains','river_valley','scrub_maquis'))
      OR (good_key = 'timber' AND terrain_type = 'forest_cedar') );
