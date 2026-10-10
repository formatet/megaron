-- Stormar som syns och driver (megaron_plan_stormar.md, Timothy 2026-10-10).
-- A storm is a unit of three connected sea hexes; each tick ONE hex steps to an
-- adjacent hex and the unit stays connected. The track is the stored outcome of
-- those steps (CLAUDE.md Events: rolled once, kept as data). Ships are hit by the
-- track, not by a die: sea_storm_progress.last_tick is the per-voyage high-water
-- mark in ticks (last_hex stays for rows written before this migration).
CREATE TABLE sea_storms (
    id         UUID     PRIMARY KEY DEFAULT gen_random_uuid(),
    world_id   UUID     NOT NULL REFERENCES worlds(id) ON DELETE CASCADE,
    heading    SMALLINT NOT NULL,
    created_tick INT    NOT NULL
);
CREATE INDEX idx_sea_storms_world ON sea_storms (world_id);

CREATE TABLE sea_storm_track (
    storm_id UUID     NOT NULL REFERENCES sea_storms(id) ON DELETE CASCADE,
    tick     INT      NOT NULL,
    slot     SMALLINT NOT NULL,
    q        INT      NOT NULL,
    r        INT      NOT NULL,
    PRIMARY KEY (storm_id, tick, slot)
);

-- Fog-of-war memory: where each Wanax last saw each storm (hexes as [[q,r],...]).
CREATE TABLE player_storm_sightings (
    player_id UUID  NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    storm_id  UUID  NOT NULL REFERENCES sea_storms(id) ON DELETE CASCADE,
    seen_tick INT   NOT NULL,
    hexes     JSONB NOT NULL,
    PRIMARY KEY (player_id, storm_id)
);

ALTER TABLE sea_storm_progress ADD COLUMN last_tick INT;
