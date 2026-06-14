-- C1: Discrete unit model — each row is one indivisible unit (cohort / vessel).
-- The old integer army columns on settlements/marching_armies are kept untouched;
-- they will be removed in C8 once clients are migrated.

CREATE TABLE units (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    world_id        UUID        NOT NULL REFERENCES worlds(id) ON DELETE CASCADE,
    owner_id        UUID        NOT NULL REFERENCES players(id) ON DELETE CASCADE,

    -- Unit identity
    type            TEXT        NOT NULL,     -- infantry|elite_infantry|chariot|priest|galley|war_galley|merchantman
    category        TEXT        NOT NULL,     -- land|naval
    size            INT         NOT NULL DEFAULT 0,   -- land: men (0–100); naval: always 1 vessel
    crew            INT         NOT NULL DEFAULT 0,   -- naval: men drawn from population; 0 for land

    -- Naval transport: UUID of a land unit riding this vessel (NULL when empty)
    cargo_unit_id   UUID        REFERENCES units(id) ON DELETE SET NULL,

    -- Lifecycle
    status          TEXT        NOT NULL DEFAULT 'forming',
    -- forming | garrison | marching | positioned | disbanded

    -- Combat stance (NULL when not in a static stance)
    stance          TEXT,
    -- fortify | storm | sentry

    -- Location: in a settlement (garrison/forming)
    settlement_id   UUID        REFERENCES settlements(id) ON DELETE SET NULL,

    -- Location: on the map (marching / positioned / sentry)
    q               INT,
    r               INT,

    -- March destination
    target_q        INT,
    target_r        INT,
    departs_at      TIMESTAMPTZ,
    arrives_at      TIMESTAMPTZ,

    -- Sentry patrol centre (when stance = 'sentry')
    sentry_q        INT,
    sentry_r        INT,

    -- Leader role label (e.g. 'dekarchos'); locked with Timothy before UI text is written
    leader_role     TEXT,

    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Lookup indexes
CREATE INDEX ON units (world_id);
CREATE INDEX ON units (owner_id, world_id);
CREATE INDEX ON units (settlement_id) WHERE settlement_id IS NOT NULL;
CREATE INDEX ON units (status, world_id) WHERE status NOT IN ('disbanded');
