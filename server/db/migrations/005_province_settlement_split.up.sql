-- Migration 005: Split provinces into provinces (terrain) + settlements (inhabited center)
-- After this migration:
--   provinces = hex tile with terrain, territory state, optional controller
--   settlements = inhabited fortress with resources, army, loyalty, owner

-- ── Step 1: Add terrain columns to provinces ────────────────────────────────
ALTER TABLE provinces
    ADD COLUMN terrain_type     TEXT NOT NULL DEFAULT 'plains',
    ADD COLUMN territory_state  TEXT NOT NULL DEFAULT 'free';

-- Populate terrain from map_tiles where available
UPDATE provinces p
SET terrain_type = COALESCE(
    (SELECT terrain FROM map_tiles WHERE world_id = p.world_id AND q = p.map_q AND r = p.map_r),
    'plains'
);

-- Provinces with a player are controlled
UPDATE provinces SET territory_state = 'controlled' WHERE player_id IS NOT NULL;

-- ── Step 2: Create settlements table ────────────────────────────────────────
CREATE TABLE settlements (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    world_id        UUID NOT NULL REFERENCES worlds(id) ON DELETE CASCADE,
    province_id     UUID NOT NULL REFERENCES provinces(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    culture_id      TEXT NOT NULL,
    owner_id        UUID REFERENCES players(id),
    kingdom_id      UUID REFERENCES kingdoms(id),
    control_type    TEXT NOT NULL DEFAULT 'capital',   -- 'capital'|'colony'|'occupied'
    founded_from    UUID,                               -- FK added after self-ref resolved below
    governor_id     UUID REFERENCES players(id),
    governor_is_ai  BOOLEAN NOT NULL DEFAULT false,
    loyalty         INT NOT NULL DEFAULT 2 CHECK (loyalty BETWEEN 1 AND 4),
    loyalty_trend   TEXT NOT NULL DEFAULT 'stable',    -- 'rising'|'stable'|'falling'
    wall_level      INT NOT NULL DEFAULT 0,
    is_capital      BOOLEAN NOT NULL DEFAULT true,
    state           TEXT NOT NULL DEFAULT 'active',    -- 'active'|'besieged'|'revolting'|'sunk'
    population      INT NOT NULL DEFAULT 100,
    -- Resources (lazy eval — same pattern as before, now on settlements)
    gold_amount     FLOAT NOT NULL DEFAULT 100,
    gold_rate       FLOAT NOT NULL DEFAULT 1.0,
    gold_cap        FLOAT NOT NULL DEFAULT 1000,
    gold_calc_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    food_amount     FLOAT NOT NULL DEFAULT 100,
    food_rate       FLOAT NOT NULL DEFAULT 2.0,
    food_cap        FLOAT NOT NULL DEFAULT 500,
    food_calc_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    lumber_amount   FLOAT NOT NULL DEFAULT 50,
    lumber_rate     FLOAT NOT NULL DEFAULT 0.5,
    lumber_cap      FLOAT NOT NULL DEFAULT 300,
    lumber_calc_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    stone_amount    FLOAT NOT NULL DEFAULT 30,
    stone_rate      FLOAT NOT NULL DEFAULT 0.3,
    stone_cap       FLOAT NOT NULL DEFAULT 200,
    stone_calc_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    iron_amount     FLOAT NOT NULL DEFAULT 0,
    iron_rate       FLOAT NOT NULL DEFAULT 0.0,
    iron_cap        FLOAT NOT NULL DEFAULT 150,
    iron_calc_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    kharis_amount   FLOAT NOT NULL DEFAULT 0,
    kharis_rate     FLOAT NOT NULL DEFAULT 0.0,
    kharis_cap      FLOAT NOT NULL DEFAULT 100,
    kharis_calc_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- Garrison army
    infantry        INT NOT NULL DEFAULT 10,
    cavalry         INT NOT NULL DEFAULT 0,
    catapult        INT NOT NULL DEFAULT 0,
    priest          INT NOT NULL DEFAULT 0,
    ship            INT NOT NULL DEFAULT 0,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (world_id, province_id)
);

ALTER TABLE settlements
    ADD CONSTRAINT fk_settlements_founded_from
    FOREIGN KEY (founded_from) REFERENCES settlements(id);

CREATE INDEX idx_settlements_owner ON settlements (owner_id, world_id);
CREATE INDEX idx_settlements_kingdom ON settlements (kingdom_id);

-- ── Step 3: Migrate province data into settlements ───────────────────────────
INSERT INTO settlements (
    world_id, province_id, name, culture_id, owner_id, kingdom_id,
    control_type, is_capital, state, population, wall_level,
    gold_amount, gold_rate, gold_cap, gold_calc_at,
    food_amount, food_rate, food_cap, food_calc_at,
    lumber_amount, lumber_rate, lumber_cap, lumber_calc_at,
    stone_amount, stone_rate, stone_cap, stone_calc_at,
    iron_amount, iron_rate, iron_cap, iron_calc_at,
    kharis_amount, kharis_rate, kharis_cap, kharis_calc_at,
    infantry, cavalry, catapult, priest, ship,
    updated_at
)
SELECT
    world_id, id,
    name, culture_id, player_id, kingdom_id,
    'capital', true,
    CASE WHEN state = 'active' THEN 'active' ELSE 'active' END,
    population, walls,
    gold_amount, gold_rate, gold_cap, gold_calc_at,
    food_amount, food_rate, food_cap, food_calc_at,
    lumber_amount, lumber_rate, lumber_cap, lumber_calc_at,
    stone_amount, stone_rate, stone_cap, stone_calc_at,
    iron_amount, iron_rate, iron_cap, iron_calc_at,
    kharis_amount, kharis_rate, kharis_cap, kharis_calc_at,
    infantry, cavalry, catapult, priest, ship,
    updated_at
FROM provinces
WHERE player_id IS NOT NULL;

-- ── Step 4: Add controller_id to provinces ───────────────────────────────────
ALTER TABLE provinces
    ADD COLUMN controller_id UUID REFERENCES settlements(id);

UPDATE provinces p
SET controller_id = (SELECT id FROM settlements WHERE province_id = p.id LIMIT 1);

-- ── Step 5: Strip inhabited columns from provinces ───────────────────────────
ALTER TABLE provinces
    DROP COLUMN player_id,
    DROP COLUMN name,
    DROP COLUMN culture_id,
    DROP COLUMN kingdom_id,
    DROP COLUMN state,
    DROP COLUMN population,
    DROP COLUMN walls,
    DROP COLUMN gold_amount,    DROP COLUMN gold_rate,    DROP COLUMN gold_cap,    DROP COLUMN gold_calc_at,
    DROP COLUMN food_amount,    DROP COLUMN food_rate,    DROP COLUMN food_cap,    DROP COLUMN food_calc_at,
    DROP COLUMN lumber_amount,  DROP COLUMN lumber_rate,  DROP COLUMN lumber_cap,  DROP COLUMN lumber_calc_at,
    DROP COLUMN stone_amount,   DROP COLUMN stone_rate,   DROP COLUMN stone_cap,   DROP COLUMN stone_calc_at,
    DROP COLUMN iron_amount,    DROP COLUMN iron_rate,    DROP COLUMN iron_cap,    DROP COLUMN iron_calc_at,
    DROP COLUMN kharis_amount,  DROP COLUMN kharis_rate,  DROP COLUMN kharis_cap,  DROP COLUMN kharis_calc_at,
    DROP COLUMN infantry,       DROP COLUMN cavalry,      DROP COLUMN catapult,
    DROP COLUMN priest,         DROP COLUMN ship,
    DROP COLUMN updated_at;

-- ── Step 6: buildings → settlements ─────────────────────────────────────────
ALTER TABLE buildings ADD COLUMN settlement_id UUID REFERENCES settlements(id);
UPDATE buildings b
SET settlement_id = (SELECT id FROM settlements WHERE province_id = b.province_id LIMIT 1);
-- Keep rows without settlement (unowned tiles had no settlement) — those are NULL for now
ALTER TABLE buildings DROP COLUMN province_id;

-- ── Step 7: build_queue → settlements ───────────────────────────────────────
ALTER TABLE build_queue ADD COLUMN settlement_id UUID REFERENCES settlements(id);
UPDATE build_queue bq
SET settlement_id = (SELECT id FROM settlements WHERE province_id = bq.province_id LIMIT 1);
ALTER TABLE build_queue DROP COLUMN province_id;

-- ── Step 8: temples → settlements ───────────────────────────────────────────
ALTER TABLE temples ADD COLUMN settlement_id UUID REFERENCES settlements(id);
UPDATE temples t
SET settlement_id = (SELECT id FROM settlements WHERE province_id = t.province_id LIMIT 1);
ALTER TABLE temples DROP COLUMN province_id;

-- ── Step 9: player_world_records — add settlement reference ──────────────────
ALTER TABLE player_world_records ADD COLUMN settlement_id UUID REFERENCES settlements(id);
UPDATE player_world_records pwr
SET settlement_id = (
    SELECT id FROM settlements
    WHERE owner_id = pwr.player_id AND world_id = pwr.world_id AND is_capital = true
    LIMIT 1
);
ALTER TABLE player_world_records DROP COLUMN province_id;

-- ── Step 10: kingdom_members — simplify (province_id → settlement ownership) ─
ALTER TABLE kingdom_members
    ADD COLUMN kingdom_bonus    TEXT NOT NULL DEFAULT 'morale',
    ADD COLUMN trade_monopoly   TEXT,
    ADD COLUMN tribute_rate     FLOAT NOT NULL DEFAULT 0.1;

ALTER TABLE kingdom_members DROP CONSTRAINT kingdom_members_pkey;
ALTER TABLE kingdom_members DROP COLUMN province_id;
ALTER TABLE kingdom_members ADD PRIMARY KEY (kingdom_id, player_id);

-- ── Step 11: kingdoms — add election/king fields ─────────────────────────────
ALTER TABLE kingdoms
    ADD COLUMN king_id              UUID REFERENCES players(id),
    ADD COLUMN king_locked_until    TIMESTAMPTZ,
    ADD COLUMN next_election_window TIMESTAMPTZ;

UPDATE kingdoms k
SET king_id = (
    SELECT player_id FROM kingdom_members
    WHERE kingdom_id = k.id AND role = 'king'
    LIMIT 1
);

-- ── Step 12: players — add AI fields ─────────────────────────────────────────
ALTER TABLE players
    ADD COLUMN is_ai            BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN ai_difficulty    TEXT NOT NULL DEFAULT 'passive';

-- ── Step 13: New tables ───────────────────────────────────────────────────────
CREATE TABLE loyalty_events (
    id              BIGSERIAL PRIMARY KEY,
    settlement_id   UUID NOT NULL REFERENCES settlements(id) ON DELETE CASCADE,
    world_id        UUID NOT NULL,
    event_type      TEXT NOT NULL,
    loyalty_delta   INT NOT NULL DEFAULT 0,
    reason          TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_loyalty_events_settlement ON loyalty_events (settlement_id, created_at);

CREATE TABLE gossip_events (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    world_id        UUID NOT NULL,
    recipient_id    UUID NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    source_region   TEXT NOT NULL,
    category        TEXT NOT NULL,
    text            TEXT NOT NULL,
    is_accurate     BOOLEAN NOT NULL DEFAULT true,
    generated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_gossip_recipient ON gossip_events (recipient_id, generated_at);

CREATE TABLE borrowed_armies (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    kingdom_id  UUID NOT NULL REFERENCES kingdoms(id) ON DELETE CASCADE,
    lender_id   UUID NOT NULL REFERENCES players(id),
    infantry    INT NOT NULL DEFAULT 0,
    cavalry     INT NOT NULL DEFAULT 0,
    catapult    INT NOT NULL DEFAULT 0,
    priest      INT NOT NULL DEFAULT 0,
    ship        INT NOT NULL DEFAULT 0,
    borrowed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    returned_at TIMESTAMPTZ
);

CREATE TABLE messengers (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    world_id        UUID NOT NULL REFERENCES worlds(id) ON DELETE CASCADE,
    sender_id       UUID NOT NULL REFERENCES players(id),
    origin_id       UUID NOT NULL REFERENCES settlements(id) ON DELETE CASCADE,
    destination_id  UUID NOT NULL REFERENCES settlements(id) ON DELETE CASCADE,
    message_text    TEXT NOT NULL,
    reply_text      TEXT,
    status          TEXT NOT NULL DEFAULT 'outbound',
    hex_q           INT NOT NULL,
    hex_r           INT NOT NULL,
    carried_by      UUID,
    sent_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    arrives_at      TIMESTAMPTZ NOT NULL
);
CREATE INDEX idx_messengers_pending ON messengers (arrives_at)
    WHERE status NOT IN ('arrived', 'lost');
