-- The deleted placements are not restored (the gubbar are in the pool, not lost).
INSERT INTO production_rules (terrain_type, building_type, good_key, rate_per_tick)
VALUES ('forest_cedar', NULL, 'timber', 1.0 / 3.0);
