-- Migration 124: map_tiles.landmass_id (megaron_plan_spawn_landmassa.md Slice 1).
--
-- Persists world.landComponents' connected-component id for every LAND tile,
-- so the join-spawn rule (Slice 2) can balance placement per landmass instead
-- of per hemisphere. Sea tiles get NULL — they are not part of any component.
-- Nullable and NOT backfilled: a reseed gives every new world a fresh map
-- with the column populated at insert time; an existing world simply has
-- landmass_id NULL on every row, and the Slice 2 spawn query falls back to
-- its pre-landmass tiebreak for those worlds rather than crashing.
ALTER TABLE map_tiles ADD COLUMN landmass_id INT;
