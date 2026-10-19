-- Placements lifted by the up migration cannot be restored (gubbar are back in
-- the pool); only the plains production rule returns.
INSERT INTO production_rules (terrain_type, building_type, good_key, rate_per_tick)
SELECT 'plains', NULL, 'livestock', 0.025 / 36
WHERE NOT EXISTS (SELECT 1 FROM production_rules WHERE good_key = 'livestock');
