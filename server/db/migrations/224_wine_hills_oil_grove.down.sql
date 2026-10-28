CREATE OR REPLACE FUNCTION silo_stored_good(p_good TEXT) RETURNS BOOLEAN LANGUAGE sql IMMUTABLE AS $$
    SELECT p_good IN ('grain','fish','oil','wine')
$$;
UPDATE settlement_goods
   SET cap = GREATEST(food_store_cap(settlement_id), LEAST(cap, settled(amount, rate, calc_tick)))
 WHERE good_key = 'wine';

UPDATE production_rules SET rate_per_tick = 0.6666666666666665
 WHERE good_key = 'oil' AND terrain_type = 'plains' AND building_type IS NULL;
UPDATE production_rules SET rate_per_tick = 0.49999999999999994
 WHERE good_key = 'oil' AND terrain_type = 'hills' AND building_type IS NULL;
UPDATE production_rules SET rate_per_tick = 0.9999999999999999
 WHERE good_key = 'oil' AND terrain_type = 'plains' AND building_type = 'olive_press';
UPDATE production_rules SET rate_per_tick = 1.333333333333333
 WHERE good_key = 'oil' AND terrain_type = 'hills' AND building_type = 'olive_press';

INSERT INTO production_rules (terrain_type, building_type, good_key, rate_per_tick) VALUES
    ('plains', NULL, 'wine', 0.49999999999999994),
    ('plains', 'winery', 'wine', 1.5),
    ('scrub_maquis', NULL, 'wine', 0.33333333333333337),
    ('scrub_maquis', 'winery', 'wine', 0.8333333333333333),
    ('river_valley', NULL, 'wine', 0.49999999999999994),
    ('river_valley', 'winery', 'wine', 1.5);
