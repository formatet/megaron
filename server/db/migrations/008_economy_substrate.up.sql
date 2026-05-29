-- Migration 008: Economic substrate — goods, settlement_goods, production rules, deposits

-- ── Ensure buildings unique index exists (province_id constraint was dropped in 005) ────
CREATE UNIQUE INDEX IF NOT EXISTS buildings_settlement_building
    ON buildings (settlement_id, building_type);

-- ── goods catalog ─────────────────────────────────────────────────────────────
CREATE TABLE goods (
    key        TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    tier       TEXT NOT NULL CHECK (tier IN ('commodity', 'manufactured')),
    category   TEXT NOT NULL CHECK (category IN ('staple', 'strategic', 'prestige', 'bulk')),
    base_value FLOAT NOT NULL DEFAULT 1.0,
    weight     FLOAT NOT NULL DEFAULT 1.0   -- for transport cost (weight × distance)
);

INSERT INTO goods (key, name, tier, category, base_value, weight) VALUES
    ('grain',  'Grain',   'commodity',    'staple',    3.0,  2.0),
    ('fish',   'Fish',    'commodity',    'staple',    2.0,  1.5),
    ('cedar',  'Cedar',   'commodity',    'bulk',      8.0,  3.0),
    ('copper', 'Copper',  'commodity',    'strategic', 6.0,  2.0),
    ('tin',    'Tin',     'commodity',    'strategic', 12.0, 2.0),
    ('wine',   'Wine',    'commodity',    'prestige',  5.0,  1.5),
    ('oil',    'Oil',     'commodity',    'prestige',  4.0,  1.5),
    ('horses', 'Horses',  'commodity',    'strategic', 15.0, 5.0),
    ('bronze', 'Bronze',  'manufactured', 'strategic', 20.0, 2.5);

-- ── settlement_goods — lazy eval, identical pattern to raw resources ────────
CREATE TABLE settlement_goods (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    settlement_id UUID NOT NULL REFERENCES settlements(id) ON DELETE CASCADE,
    good_key      TEXT NOT NULL REFERENCES goods(key),
    amount        FLOAT NOT NULL DEFAULT 0,
    rate          FLOAT NOT NULL DEFAULT 0,  -- per minute
    cap           FLOAT NOT NULL DEFAULT 100,
    calc_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (settlement_id, good_key)
);
CREATE INDEX idx_settlement_goods ON settlement_goods (settlement_id);

-- ── production_rules: terräng + byggnad → vara + rate ─────────────────────
CREATE TABLE production_rules (
    id               SERIAL PRIMARY KEY,
    terrain_type     TEXT,           -- NULL = matches any terrain
    building_type    TEXT,           -- NULL = terrain-only rule (no building required)
    good_key         TEXT NOT NULL REFERENCES goods(key),
    rate_per_min     FLOAT NOT NULL,
    requires_deposit TEXT            -- NULL | 'copper' | 'tin'
);

-- Terrain-only baseline (active from day 1, no building needed)
INSERT INTO production_rules (terrain_type, building_type, good_key, rate_per_min, requires_deposit) VALUES
    ('coast',    NULL, 'fish',   0.04,  NULL),
    ('plains',   NULL, 'grain',  0.03,  NULL),
    ('plains',   NULL, 'oil',    0.02,  NULL),
    ('hills',    NULL, 'wine',   0.02,  NULL),
    ('hills',    NULL, 'oil',    0.015, NULL),
    ('hills',    NULL, 'copper', 0.02,  'copper'),
    ('mountain', NULL, 'tin',    0.01,  'tin');

-- Building-augmented production (added when building completes)
INSERT INTO production_rules (terrain_type, building_type, good_key, rate_per_min, requires_deposit) VALUES
    ('plains',   'farm',       'grain',  0.05,  NULL),
    ('plains',   'farm',       'oil',    0.03,  NULL),
    ('hills',    'farm',       'wine',   0.04,  NULL),
    ('forest',   'lumbermill', 'cedar',  0.05,  NULL),
    ('coast',    'harbour',    'fish',   0.04,  NULL),
    ('hills',    'mine',       'copper', 0.04,  'copper'),
    ('mountain', 'mine',       'tin',    0.025, 'tin'),
    (NULL,       'stable',     'horses', 0.02,  NULL);

-- ── Deposit flags on provinces and map_tiles ──────────────────────────────
ALTER TABLE provinces
    ADD COLUMN copper_deposit BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN tin_deposit    BOOLEAN NOT NULL DEFAULT false;

ALTER TABLE map_tiles
    ADD COLUMN copper_deposit BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN tin_deposit    BOOLEAN NOT NULL DEFAULT false;
