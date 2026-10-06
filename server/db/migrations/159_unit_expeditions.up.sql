-- Upptäckarexpeditionen (megaron_plan_upptackarexpeditionen.md, 2026-10-06):
-- an explore order covers an AREA (a chosen hex + radius) for a chosen number
-- of ticks, walking leg by leg to the nearest unexplored hex and turning home
-- by half the length. One row per expedition in flight.
--
-- leg_arrive_tick is the self-validation: a row only governs the unit while it
-- equals units.arrive_tick. Recall/redirect/a new order change arrive_tick, so
-- a stale row is ignored (and deleted) instead of steering a unit that has
-- been given another order — no other march writer needs to know this table.
CREATE TABLE unit_expeditions (
    unit_id         UUID    PRIMARY KEY REFERENCES units(id) ON DELETE CASCADE,
    world_id        UUID    NOT NULL REFERENCES worlds(id) ON DELETE CASCADE,
    area_q          INT     NOT NULL,
    area_r          INT     NOT NULL,
    length_ticks    INT     NOT NULL CHECK (length_ticks > 0),
    start_tick      INT     NOT NULL,
    turn_tick       INT     NOT NULL,
    leg_arrive_tick INT     NOT NULL,
    homeward        BOOLEAN NOT NULL DEFAULT false,
    turn_reason     TEXT    CHECK (turn_reason IN ('half_time', 'area_known', 'no_path')),
    furthest        INT     NOT NULL DEFAULT 0
);

-- Every hex the expedition itself saw, for the homecoming report. "Seen", not
-- "new": the player's own map reads may have recorded some first.
CREATE TABLE unit_expedition_seen (
    unit_id UUID NOT NULL REFERENCES unit_expeditions(unit_id) ON DELETE CASCADE,
    q       INT  NOT NULL,
    r       INT  NOT NULL,
    PRIMARY KEY (unit_id, q, r)
);
