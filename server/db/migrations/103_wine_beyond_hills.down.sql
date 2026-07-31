-- Revert 103: remove exactly the five rows the up migration added.
-- Hills' own wine rows (migrations 008 + 019) are untouched — this migration
-- never touched them.

DELETE FROM production_rules
 WHERE good_key = 'wine'
   AND terrain_type IN ('plains', 'scrub_maquis')
   AND (
       (terrain_type = 'plains'       AND building_type IS NULL     AND rate_per_tick = 0.6) OR
       (terrain_type = 'plains'       AND building_type = 'farm'    AND rate_per_tick = 1.2) OR
       (terrain_type = 'plains'       AND building_type = 'winery'  AND rate_per_tick = 1.8) OR
       (terrain_type = 'scrub_maquis' AND building_type IS NULL     AND rate_per_tick = 0.4) OR
       (terrain_type = 'scrub_maquis' AND building_type = 'winery'  AND rate_per_tick = 1.0)
   );
