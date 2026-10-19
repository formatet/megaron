DELETE FROM production_rules
WHERE terrain_type IN ('scrub_maquis','hills','river_valley')
  AND building_type IS NULL AND good_key = 'timber';
