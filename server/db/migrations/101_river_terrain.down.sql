-- Rollback of 101: restore the land-coastal fish rules, drop the water-terrain
-- fish rules. Does NOT revert the coastal backfill (a superset of TRUE values
-- that were already correct under the old definition is harmless to leave set)
-- and does NOT touch any river/river_valley terrain values — this migration's
-- up side never wrote terrain, only production_rules and the coastal flag.
DELETE FROM production_rules
 WHERE good_key = 'fish' AND terrain_type IN ('coastal_sea', 'river', 'deep_sea');

INSERT INTO production_rules (terrain_type, building_type, good_key, rate_per_tick, requires_deposit, requires_coastal) VALUES
    (NULL, NULL,      'fish', 2.4, NULL, TRUE),
    (NULL, 'harbour', 'fish', 2.4, NULL, TRUE);
