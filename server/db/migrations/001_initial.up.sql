-- Event sourcing core
CREATE TABLE events (
    id          BIGSERIAL PRIMARY KEY,
    stream_id   UUID NOT NULL,
    stream_type TEXT NOT NULL,
    event_type  TEXT NOT NULL,
    payload     JSONB NOT NULL,
    causation   BIGINT REFERENCES events(id),
    world_id    UUID NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_events_stream ON events (stream_id, id);
CREATE INDEX idx_events_world ON events (world_id, created_at);

-- Durable job queue for timed game events
CREATE TABLE scheduled_events (
    id              BIGSERIAL PRIMARY KEY,
    world_id        UUID NOT NULL,
    event_type      TEXT NOT NULL,
    payload         JSONB NOT NULL,
    process_after   TIMESTAMPTZ NOT NULL,
    processed_at    TIMESTAMPTZ,
    failed_at       TIMESTAMPTZ,
    attempts        INT NOT NULL DEFAULT 0
);

CREATE INDEX idx_scheduled_pending ON scheduled_events (process_after)
    WHERE processed_at IS NULL;

-- Players
CREATE TABLE players (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    username      TEXT NOT NULL UNIQUE,
    email         TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    era_count     INT NOT NULL DEFAULT 0,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Worlds
CREATE TABLE worlds (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name              TEXT NOT NULL UNIQUE,
    state             TEXT NOT NULL DEFAULT 'forming',
    prestige          INT NOT NULL DEFAULT 0,
    era_number        INT NOT NULL DEFAULT 1,
    era_started_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    max_provinces     INT NOT NULL DEFAULT 100,
    min_era_weeks     INT NOT NULL DEFAULT 10,
    max_era_weeks     INT NOT NULL DEFAULT 25,
    kingdom_min_size  INT NOT NULL DEFAULT 3,
    kingdom_max_size  INT NOT NULL DEFAULT 12,
    map_seed          BIGINT NOT NULL DEFAULT 0,
    map_width         INT NOT NULL DEFAULT 40,
    map_height        INT NOT NULL DEFAULT 30,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Map tiles — generated procedurally from seed, stored after generation
CREATE TABLE map_tiles (
    world_id    UUID NOT NULL REFERENCES worlds(id) ON DELETE CASCADE,
    q           INT NOT NULL,
    r           INT NOT NULL,
    terrain     TEXT NOT NULL, -- "plains" | "forest" | "hills" | "mountain" | "coast" | "sea"
    fertility   FLOAT NOT NULL DEFAULT 0.5,
    mineral     FLOAT NOT NULL DEFAULT 0.5,
    PRIMARY KEY (world_id, q, r)
);

-- Provinces
CREATE TABLE provinces (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    world_id    UUID NOT NULL REFERENCES worlds(id) ON DELETE CASCADE,
    player_id   UUID REFERENCES players(id),
    name        TEXT NOT NULL,
    culture_id  TEXT NOT NULL,
    kingdom_id  UUID,
    map_q       INT NOT NULL,
    map_r       INT NOT NULL,
    state       TEXT NOT NULL DEFAULT 'active',
    population  INT NOT NULL DEFAULT 100,
    walls       INT NOT NULL DEFAULT 0,
    -- Lazy resource ledger: gold
    gold_amount        FLOAT NOT NULL DEFAULT 100,
    gold_rate          FLOAT NOT NULL DEFAULT 1.0,
    gold_cap           FLOAT NOT NULL DEFAULT 1000,
    gold_calc_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- food
    food_amount        FLOAT NOT NULL DEFAULT 100,
    food_rate          FLOAT NOT NULL DEFAULT 2.0,
    food_cap           FLOAT NOT NULL DEFAULT 500,
    food_calc_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- lumber
    lumber_amount      FLOAT NOT NULL DEFAULT 50,
    lumber_rate        FLOAT NOT NULL DEFAULT 0.5,
    lumber_cap         FLOAT NOT NULL DEFAULT 300,
    lumber_calc_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- stone
    stone_amount       FLOAT NOT NULL DEFAULT 30,
    stone_rate         FLOAT NOT NULL DEFAULT 0.3,
    stone_cap          FLOAT NOT NULL DEFAULT 200,
    stone_calc_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- iron
    iron_amount        FLOAT NOT NULL DEFAULT 0,
    iron_rate          FLOAT NOT NULL DEFAULT 0.0,
    iron_cap           FLOAT NOT NULL DEFAULT 150,
    iron_calc_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- mana
    mana_amount        FLOAT NOT NULL DEFAULT 0,
    mana_rate          FLOAT NOT NULL DEFAULT 0.0,
    mana_cap           FLOAT NOT NULL DEFAULT 100,
    mana_calc_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- army
    infantry    INT NOT NULL DEFAULT 10,
    cavalry     INT NOT NULL DEFAULT 0,
    catapult    INT NOT NULL DEFAULT 0,
    wizard      INT NOT NULL DEFAULT 0,
    ship        INT NOT NULL DEFAULT 0,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (world_id, map_q, map_r)
);

-- Marching armies
CREATE TABLE marching_armies (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    world_id         UUID NOT NULL REFERENCES worlds(id) ON DELETE CASCADE,
    origin_id        UUID NOT NULL REFERENCES provinces(id),
    target_id        UUID NOT NULL REFERENCES provinces(id),
    infantry         INT NOT NULL DEFAULT 0,
    cavalry          INT NOT NULL DEFAULT 0,
    catapult         INT NOT NULL DEFAULT 0,
    wizard           INT NOT NULL DEFAULT 0,
    ship             INT NOT NULL DEFAULT 0,
    intent           TEXT NOT NULL DEFAULT 'attack',
    support_target   UUID REFERENCES provinces(id),
    departs_at       TIMESTAMPTZ NOT NULL,
    arrives_at       TIMESTAMPTZ NOT NULL,
    resolved         BOOLEAN NOT NULL DEFAULT false,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_marching_arrives ON marching_armies (arrives_at) WHERE resolved = false;

-- Buildings
CREATE TABLE buildings (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    province_id UUID NOT NULL REFERENCES provinces(id) ON DELETE CASCADE,
    type        TEXT NOT NULL,
    level       INT NOT NULL DEFAULT 1,
    built_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Build queue
CREATE TABLE build_queue (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    province_id     UUID NOT NULL REFERENCES provinces(id) ON DELETE CASCADE,
    world_id        UUID NOT NULL,
    building_type   TEXT NOT NULL,
    complete_at     TIMESTAMPTZ NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Kingdoms
CREATE TABLE kingdoms (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    world_id    UUID NOT NULL REFERENCES worlds(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    prestige    INT NOT NULL DEFAULT 0,
    -- treasury
    gold_amount        FLOAT NOT NULL DEFAULT 0,
    gold_rate          FLOAT NOT NULL DEFAULT 0,
    gold_cap           FLOAT NOT NULL DEFAULT 5000,
    gold_calc_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (world_id, name)
);

CREATE TABLE kingdom_members (
    kingdom_id  UUID NOT NULL REFERENCES kingdoms(id) ON DELETE CASCADE,
    province_id UUID NOT NULL REFERENCES provinces(id) ON DELETE CASCADE,
    player_id   UUID NOT NULL REFERENCES players(id),
    role        TEXT NOT NULL DEFAULT 'member',
    joined_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (kingdom_id, province_id)
);

-- Kingdom invitations
CREATE TABLE kingdom_invitations (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    kingdom_id  UUID NOT NULL REFERENCES kingdoms(id) ON DELETE CASCADE,
    province_id UUID NOT NULL REFERENCES provinces(id) ON DELETE CASCADE,
    invited_by  UUID NOT NULL REFERENCES players(id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at  TIMESTAMPTZ NOT NULL,
    accepted_at TIMESTAMPTZ
);

-- Religion
CREATE TABLE temples (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    province_id  UUID NOT NULL REFERENCES provinces(id) ON DELETE CASCADE,
    pantheon_id  TEXT NOT NULL,
    level        INT NOT NULL DEFAULT 1,
    local_power  FLOAT NOT NULL DEFAULT 0.5,
    priest_id    UUID REFERENCES players(id),
    built_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE divine_interventions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    world_id        UUID NOT NULL REFERENCES worlds(id) ON DELETE CASCADE,
    pantheon_id     TEXT NOT NULL,
    type            TEXT NOT NULL,
    target_id       UUID NOT NULL,
    probability     FLOAT NOT NULL DEFAULT 0.5,
    triggered_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Player-world membership
CREATE TABLE player_world_records (
    player_id   UUID NOT NULL REFERENCES players(id),
    world_id    UUID NOT NULL REFERENCES worlds(id) ON DELETE CASCADE,
    province_id UUID REFERENCES provinces(id),
    status      TEXT NOT NULL DEFAULT 'active',
    epithets    TEXT[] NOT NULL DEFAULT '{}',
    joined_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (player_id, world_id)
);

-- JWT refresh tokens
CREATE TABLE refresh_tokens (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    player_id   UUID NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    token_hash  TEXT NOT NULL UNIQUE,
    expires_at  TIMESTAMPTZ NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
