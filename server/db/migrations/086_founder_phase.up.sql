-- Migration 086: founder_phase — the Nomadic Host's state before a capital exists.
--
-- A new player joins as a Nomadic Host: a token on the map (units row,
-- type='nomadic_host') carrying 4 000 people and a locked, finite store of grain
-- and silver. They wander, look for a site, and found exactly one capital — at
-- which point the host dissolves permanently and this row goes inactive.
--
-- Design: temenos_nomadic_host_plan.md · Build: temenos_nomadic_host_bygg.md (B1/B2)
--
-- Two decisions are load-bearing here:
--
-- 1. The store is a lazy tuple, not a new rationing system. (amount, rate,
--    calc_tick) is exactly the shape settlement_goods already uses, so the same
--    settled(amount, rate, calc_tick) SQL function drains it — with a NEGATIVE
--    rate. Nothing ticks it; it is derived at read. The deadline (grain = 0) is
--    therefore also derived — calc_tick + amount/-rate — and never stored, so a
--    world that changes tempo cannot desync it from the clock.
--
-- 2. The population lives HERE, not in units.size. units.size is 0–100 for land
--    and 1 for naval; 4 000 people do not fit and do not mean the same thing.
--    The host token is size=1: one movable marker.
--
-- Soldiers are separate from population (decision Timothy 2026-07-15): the two
-- starter spearmen cohorts (200 men) sit ON TOP of these 4 000 civilians. Their
-- upkeep is folded into grain_rate/silver_rate below while the phase is active —
-- combat.UpkeepHandler must skip them until founding, or they pay twice (B3).

CREATE TABLE founder_phase (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    world_id      UUID NOT NULL REFERENCES worlds(id) ON DELETE CASCADE,
    owner_id      UUID NOT NULL REFERENCES players(id),
    host_unit_id  UUID REFERENCES units(id) ON DELETE SET NULL,

    -- Civilians carried by the host. Becomes settlements.population at founding.
    population    INT NOT NULL,

    -- Locked store. rate is per TICK and negative (drain). Read via
    -- settled(amount, rate, calc_tick); re-anchor amount+calc_tick on any write.
    grain_amount  DOUBLE PRECISION NOT NULL,
    grain_rate    DOUBLE PRECISION NOT NULL,
    silver_amount DOUBLE PRECISION NOT NULL,
    silver_rate   DOUBLE PRECISION NOT NULL,
    calc_tick     INT NOT NULL DEFAULT 0,

    -- active=false + founded_tick set == the design's one-shot flag: a player can
    -- never obtain a second host. The row is kept (not deleted) as the record of
    -- how the player started.
    active        BOOLEAN NOT NULL DEFAULT true,
    founded_tick  INT,

    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- One founder phase per player per world, ever.
    UNIQUE (world_id, owner_id)
);

-- The hot path is "does this player still lack a capital?" (every host command)
-- and "which units belong to an active founder phase?" (the upkeep exclusion, B3).
CREATE INDEX idx_founder_phase_active ON founder_phase (world_id, owner_id) WHERE active;
CREATE INDEX idx_founder_phase_host_unit ON founder_phase (host_unit_id);
