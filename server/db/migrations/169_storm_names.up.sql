-- Stormnamn (megaron_plan_stormnamn.md, Timothy 2026-10-10): the first Wanax whose ship
-- is struck by a storm may name it, once; the name stays with the storm as it drifts.
-- named_tick survives an admin clearing the name, so a cleared (foul) name cannot be set again
-- by the same player.
ALTER TABLE sea_storms
    ADD COLUMN name         TEXT,
    ADD COLUMN claimed_by   UUID REFERENCES players(id) ON DELETE SET NULL,
    ADD COLUMN claimed_tick INT,
    ADD COLUMN named_tick   INT;
CREATE UNIQUE INDEX idx_sea_storms_name ON sea_storms (world_id, lower(name)) WHERE name IS NOT NULL;
