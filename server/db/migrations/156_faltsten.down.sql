DELETE FROM production_rules
 WHERE building_type IS NULL AND good_key = 'stone'
   AND terrain_type IN ('plains','river_valley','river_delta','forest_olive_grove',
                        'forest_cedar','scrub_maquis','semi_desert','mountain_red');
