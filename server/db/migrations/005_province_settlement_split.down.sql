-- Reverse migration 005: collapse settlements back into provinces.
-- This drops ALL settlement data — for dev reset only.

-- Step 13 reversal: drop new tables
DROP TABLE IF EXISTS messengers;
DROP TABLE IF EXISTS borrowed_armies;
DROP TABLE IF EXISTS gossip_events;
DROP TABLE IF EXISTS loyalty_events;

-- Step 12 reversal: players
ALTER TABLE players
    DROP COLUMN IF EXISTS is_ai,
    DROP COLUMN IF EXISTS ai_difficulty;

-- Step 11 reversal: kingdoms
ALTER TABLE kingdoms
    DROP COLUMN IF EXISTS king_id,
    DROP COLUMN IF EXISTS king_locked_until,
    DROP COLUMN IF EXISTS next_election_window;

-- Step 10 reversal: kingdom_members — restore province_id PK
ALTER TABLE kingdom_members DROP CONSTRAINT IF EXISTS kingdom_members_pkey;
ALTER TABLE kingdom_members
    ADD COLUMN province_id UUID REFERENCES provinces(id),
    DROP COLUMN IF EXISTS kingdom_bonus,
    DROP COLUMN IF EXISTS trade_monopoly,
    DROP COLUMN IF EXISTS tribute_rate;
ALTER TABLE kingdom_members ADD PRIMARY KEY (kingdom_id, player_id, province_id);

-- Step 9 reversal: player_world_records
ALTER TABLE player_world_records
    ADD COLUMN province_id UUID REFERENCES provinces(id);
UPDATE player_world_records pwr
SET province_id = (SELECT province_id FROM settlements WHERE id = pwr.settlement_id LIMIT 1);
ALTER TABLE player_world_records DROP COLUMN IF EXISTS settlement_id;

-- Step 8 reversal: temples
ALTER TABLE temples ADD COLUMN province_id UUID REFERENCES provinces(id);
UPDATE temples t
SET province_id = (SELECT province_id FROM settlements WHERE id = t.settlement_id LIMIT 1);
ALTER TABLE temples DROP COLUMN IF EXISTS settlement_id;

-- Step 7 reversal: build_queue
ALTER TABLE build_queue ADD COLUMN province_id UUID REFERENCES provinces(id);
UPDATE build_queue bq
SET province_id = (SELECT province_id FROM settlements WHERE id = bq.settlement_id LIMIT 1);
ALTER TABLE build_queue DROP COLUMN IF EXISTS settlement_id;

-- Step 6 reversal: buildings
ALTER TABLE buildings ADD COLUMN province_id UUID REFERENCES provinces(id);
UPDATE buildings b
SET province_id = (SELECT province_id FROM settlements WHERE id = b.settlement_id LIMIT 1);
ALTER TABLE buildings DROP COLUMN IF EXISTS settlement_id;

-- Step 4/5 reversal: restore province columns from settlements data
ALTER TABLE provinces DROP COLUMN IF EXISTS controller_id;

ALTER TABLE provinces
    ADD COLUMN player_id       UUID REFERENCES players(id),
    ADD COLUMN name            TEXT NOT NULL DEFAULT '',
    ADD COLUMN culture_id      TEXT NOT NULL DEFAULT 'akhaier',
    ADD COLUMN kingdom_id      UUID REFERENCES kingdoms(id),
    ADD COLUMN state           TEXT NOT NULL DEFAULT 'active',
    ADD COLUMN population      INT  NOT NULL DEFAULT 100,
    ADD COLUMN walls           INT  NOT NULL DEFAULT 0,
    ADD COLUMN gold_amount     FLOAT NOT NULL DEFAULT 100,
    ADD COLUMN gold_rate       FLOAT NOT NULL DEFAULT 1.0,
    ADD COLUMN gold_cap        FLOAT NOT NULL DEFAULT 1000,
    ADD COLUMN gold_calc_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    ADD COLUMN food_amount     FLOAT NOT NULL DEFAULT 100,
    ADD COLUMN food_rate       FLOAT NOT NULL DEFAULT 2.0,
    ADD COLUMN food_cap        FLOAT NOT NULL DEFAULT 500,
    ADD COLUMN food_calc_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    ADD COLUMN lumber_amount   FLOAT NOT NULL DEFAULT 50,
    ADD COLUMN lumber_rate     FLOAT NOT NULL DEFAULT 0.5,
    ADD COLUMN lumber_cap      FLOAT NOT NULL DEFAULT 300,
    ADD COLUMN lumber_calc_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    ADD COLUMN stone_amount    FLOAT NOT NULL DEFAULT 30,
    ADD COLUMN stone_rate      FLOAT NOT NULL DEFAULT 0.3,
    ADD COLUMN stone_cap       FLOAT NOT NULL DEFAULT 200,
    ADD COLUMN stone_calc_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    ADD COLUMN iron_amount     FLOAT NOT NULL DEFAULT 0,
    ADD COLUMN iron_rate       FLOAT NOT NULL DEFAULT 0.0,
    ADD COLUMN iron_cap        FLOAT NOT NULL DEFAULT 150,
    ADD COLUMN iron_calc_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    ADD COLUMN kharis_amount   FLOAT NOT NULL DEFAULT 0,
    ADD COLUMN kharis_rate     FLOAT NOT NULL DEFAULT 0.0,
    ADD COLUMN kharis_cap      FLOAT NOT NULL DEFAULT 100,
    ADD COLUMN kharis_calc_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    ADD COLUMN infantry        INT NOT NULL DEFAULT 0,
    ADD COLUMN cavalry         INT NOT NULL DEFAULT 0,
    ADD COLUMN catapult        INT NOT NULL DEFAULT 0,
    ADD COLUMN priest          INT NOT NULL DEFAULT 0,
    ADD COLUMN ship            INT NOT NULL DEFAULT 0,
    ADD COLUMN updated_at      TIMESTAMPTZ NOT NULL DEFAULT now();

-- Restore province data from settlements
UPDATE provinces p
SET
    player_id     = s.owner_id,
    name          = s.name,
    culture_id    = s.culture_id::TEXT,
    kingdom_id    = s.kingdom_id,
    state         = 'active',
    population    = s.population,
    walls         = s.wall_level,
    gold_amount   = s.gold_amount,   gold_rate   = s.gold_rate,   gold_cap   = s.gold_cap,   gold_calc_at   = s.gold_calc_at,
    food_amount   = s.food_amount,   food_rate   = s.food_rate,   food_cap   = s.food_cap,   food_calc_at   = s.food_calc_at,
    lumber_amount = s.lumber_amount, lumber_rate = s.lumber_rate, lumber_cap = s.lumber_cap, lumber_calc_at = s.lumber_calc_at,
    stone_amount  = s.stone_amount,  stone_rate  = s.stone_rate,  stone_cap  = s.stone_cap,  stone_calc_at  = s.stone_calc_at,
    iron_amount   = s.iron_amount,   iron_rate   = s.iron_rate,   iron_cap   = s.iron_cap,   iron_calc_at   = s.iron_calc_at,
    kharis_amount = s.kharis_amount, kharis_rate = s.kharis_rate, kharis_cap = s.kharis_cap, kharis_calc_at = s.kharis_calc_at,
    infantry      = s.infantry,
    cavalry       = s.cavalry,
    catapult      = s.catapult,
    priest        = s.priest,
    ship          = s.ship,
    updated_at    = s.updated_at
FROM settlements s
WHERE s.province_id = p.id;

-- Step 3 reversal: drop settlements (data already merged back)
DROP TABLE IF EXISTS settlements;

-- Step 1 reversal: remove terrain columns from provinces
ALTER TABLE provinces
    DROP COLUMN IF EXISTS terrain_type,
    DROP COLUMN IF EXISTS territory_state;
