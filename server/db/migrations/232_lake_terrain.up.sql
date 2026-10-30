-- Migration 232: terrain `lake` (insjöar slice 2, Timothy 2026-10-30,
-- temenos_terrain.md §Insjöar). A lake is sea terrain that does not reach the
-- world's edge through sea; rivers do not connect. Same rule as
-- world.MarkLakes for new worlds.

-- 1. The water owners learn `lake` (mirrors province.IsSea / IsNavigable).
CREATE OR REPLACE FUNCTION terrain_is_sea(t text) RETURNS boolean
LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$ SELECT t IN ('coastal_sea','deep_sea','lake') $$;

-- The Thalassa itself (province.IsThalassa): what a shipyard's waterway must reach.
CREATE OR REPLACE FUNCTION terrain_is_thalassa(t text) RETURNS boolean
LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$ SELECT t IN ('coastal_sea','deep_sea') $$;

CREATE OR REPLACE FUNCTION terrain_is_navigable(t text) RETURNS boolean
LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$ SELECT t IN ('coastal_sea','deep_sea','lake','river','river_ford') $$;

-- 2. Flood fill from the edge. An edge hex has fewer than six neighbours on the map.
CREATE TEMP TABLE lake_hexes AS
WITH RECURSIVE sea AS (
  SELECT world_id, q, r FROM map_tiles WHERE terrain IN ('coastal_sea','deep_sea')
), edge AS (
  SELECT s.world_id, s.q, s.r FROM sea s
   WHERE (SELECT count(*) FROM map_tiles n
           WHERE n.world_id = s.world_id
             AND (n.q, n.r) IN ((s.q+1,s.r),(s.q-1,s.r),(s.q,s.r+1),(s.q,s.r-1),(s.q+1,s.r-1),(s.q-1,s.r+1))) < 6
), main AS (
  SELECT world_id, q, r FROM edge
  UNION
  SELECT s.world_id, s.q, s.r FROM main m
    JOIN sea s ON s.world_id = m.world_id
     AND (s.q, s.r) IN ((m.q+1,m.r),(m.q-1,m.r),(m.q,m.r+1),(m.q,m.r-1),(m.q+1,m.r-1),(m.q-1,m.r+1))
)
SELECT s.world_id, s.q, s.r FROM sea s
 WHERE NOT EXISTS (SELECT 1 FROM main m WHERE m.world_id = s.world_id AND m.q = s.q AND m.r = s.r);

UPDATE map_tiles mt SET terrain = 'lake'
  FROM lake_hexes l WHERE mt.world_id = l.world_id AND mt.q = l.q AND mt.r = l.r;

UPDATE provinces p SET terrain_type = 'lake'
  FROM lake_hexes l WHERE p.world_id = l.world_id AND p.map_q = l.q AND p.map_r = l.r;

DROP TABLE lake_hexes;

-- Coastal needs no backfill: since mig 101 it means "beside any water", and every
-- lake neighbour already bordered the coastal_sea the lake was.

-- 3. Fishing: a lake fishes like coastal water (Timothy 2026-10-30). Read from the
-- table, never restated (mig 108's rule).
INSERT INTO production_rules (terrain_type, building_type, good_key, rate_per_tick, requires_deposit, requires_coastal)
SELECT 'lake', building_type, good_key, rate_per_tick, requires_deposit, requires_coastal
  FROM production_rules
 WHERE terrain_type = 'coastal_sea' AND good_key = 'fish';
