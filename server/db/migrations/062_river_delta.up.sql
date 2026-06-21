-- Migration 062: river_delta terrain — highest grain in the game, coastal.
-- The river_delta terrain type is added as a pure TEXT value (terrain column is TEXT NOT NULL,
-- not a Postgres enum), so no ALTER TYPE is needed — just production rules.
--
-- Design invariant: river_delta = geographic "honey trap" (enormous grain + coastal exposure).
-- Rate: base 0.08/min (> river_valley 0.05), with farm 0.20/min.
-- Movement: 0.75 h/hex (like plains/river_valley) — flat, open delta.
-- Source: temenos_mapgen_v4.md §C, temenos_reseed_salvo.md WP3.

INSERT INTO production_rules (terrain_type, building_type, good_key, rate_per_min)
VALUES
  ('river_delta', NULL,   'grain', 0.08),   -- base: highest grain in game
  ('river_delta', 'farm', 'grain', 0.20);   -- with farm: even richer
