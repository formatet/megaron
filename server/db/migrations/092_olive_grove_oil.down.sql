-- Revert 092: remove the olive grove's oil rules.

DELETE FROM production_rules
 WHERE terrain_type = 'forest_olive_grove'
   AND good_key = 'oil'
   AND (building_type IS NULL OR building_type = 'olive_press');
