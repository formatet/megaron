-- Timothy 2026-10-28 (H1/H2): wine is not food and grows only on hills; oil
-- outside the olive grove is a third of the grove's, as timber is outside forest.
DELETE FROM production_rules
 WHERE good_key = 'wine' AND terrain_type IN ('plains', 'river_valley', 'scrub_maquis');

UPDATE production_rules SET rate_per_tick = 0.33
 WHERE good_key = 'oil' AND terrain_type IN ('plains', 'hills') AND building_type IS NULL;
UPDATE production_rules SET rate_per_tick = 0.55
 WHERE good_key = 'oil' AND terrain_type IN ('plains', 'hills') AND building_type = 'olive_press';

-- Wine leaves the silo's food store and takes the non-food cap; no stock is cut.
CREATE OR REPLACE FUNCTION silo_stored_good(p_good TEXT) RETURNS BOOLEAN LANGUAGE sql IMMUTABLE AS $$
    SELECT p_good IN ('grain','fish','oil')
$$;
UPDATE settlement_goods SET cap = 1000000 WHERE good_key = 'wine' AND cap < 1000000;
