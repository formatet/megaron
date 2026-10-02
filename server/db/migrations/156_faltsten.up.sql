-- Fältsten (megaron_plan_byggkostnader.md steg 0, Timothy 2026-10-02): mig 155 took
-- away the terrain-free quarry row, so only hills/limestone gave stone and 7 of 10
-- landmasses had none. Fieldstone is on every land hex at half the standard rate;
-- the stonequarry multiplies it (rule lives in economy/hex_rules.go).
INSERT INTO production_rules (terrain_type, building_type, good_key, rate_per_tick, requires_coastal)
SELECT t, NULL, 'stone', 0.5, false
  FROM unnest(ARRAY['plains','river_valley','river_delta','forest_olive_grove',
                    'forest_cedar','scrub_maquis','semi_desert','mountain_red']) AS t;
