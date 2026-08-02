-- Revert 105: remove exactly the three rows the up migration added.
-- Hills' and plains'/scrub_maquis' own wine rows (migrations 008, 019, 103)
-- are untouched — this migration never touched them.

DELETE FROM production_rules
 WHERE good_key = 'wine'
   AND terrain_type = 'river_valley'
   AND (
       (building_type IS NULL    AND rate_per_tick = 0.6) OR
       (building_type = 'farm'   AND rate_per_tick = 1.2) OR
       (building_type = 'winery' AND rate_per_tick = 1.8)
   );
