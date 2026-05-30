-- Migration 018: Merge economy systems
-- Drop food/lumber/stone as settlement columns — everything producible moves to settlement_goods.
-- gold and kharis stay as columns (gold = currency, kharis = divine relation).
-- New goods: grain (←food), cedar (←lumber), stone (←stone).

-- ── Add stone to goods catalog ─────────────────────────────────────────────
INSERT INTO goods (key, name, tier, category, base_value, weight) VALUES
    ('stone', 'Stone', 'commodity', 'bulk', 2.0, 3.0)
ON CONFLICT (key) DO NOTHING;

-- ── Update building production_rules to match old BuildingSpec rate values ─
-- Farm→grain was rate_per_min=0.05 (production_rule) PLUS FoodRate=2.0 (column bonus).
-- After merge, farm→grain = 2.0 total (the column-level bonus was the main effect).
UPDATE production_rules SET rate_per_min = 2.0
    WHERE building_type = 'farm' AND good_key = 'grain';

-- Lumbermill→cedar same: was 0.05 rule + LumberRate=2.0 column bonus.
UPDATE production_rules SET rate_per_min = 2.0
    WHERE building_type = 'lumbermill' AND good_key = 'cedar';

-- ── Add terrain-baseline rules for cedar and stone ─────────────────────────
INSERT INTO production_rules (terrain_type, building_type, good_key, rate_per_min, requires_deposit) VALUES
    ('forest',   NULL,          'cedar', 0.02, NULL),
    ('hills',    NULL,          'stone', 0.01, NULL),
    ('mountain', NULL,          'stone', 0.02, NULL);

-- ── Add building production_rules for stone (mine and stonequarry) ─────────
-- Mine was StoneRate=1.0 (column bonus), stonequarry StoneRate=2.0.
INSERT INTO production_rules (terrain_type, building_type, good_key, rate_per_min, requires_deposit) VALUES
    (NULL, 'mine',        'stone', 1.0, NULL),
    (NULL, 'stonequarry', 'stone', 2.0, NULL);

-- ── Migrate food → grain in settlement_goods ───────────────────────────────
-- Adds existing food stock + rate to any existing grain row (or creates new row).
INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_at)
SELECT
    s.id,
    'grain',
    GREATEST(0, s.food_amount + EXTRACT(EPOCH FROM (now() - s.food_calc_at)) / 60 * s.food_rate),
    s.food_rate,
    GREATEST(COALESCE(s.food_cap, 1000), 1000),
    now()
FROM settlements s
ON CONFLICT (settlement_id, good_key) DO UPDATE SET
    amount  = settlement_goods.amount
                + GREATEST(0, EXCLUDED.amount),
    rate    = settlement_goods.rate + EXCLUDED.rate,
    cap     = GREATEST(settlement_goods.cap, EXCLUDED.cap),
    calc_at = now();

-- ── Migrate lumber → cedar in settlement_goods ─────────────────────────────
INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_at)
SELECT
    s.id,
    'cedar',
    GREATEST(0, s.lumber_amount + EXTRACT(EPOCH FROM (now() - s.lumber_calc_at)) / 60 * s.lumber_rate),
    s.lumber_rate,
    GREATEST(COALESCE(s.lumber_cap, 500), 500),
    now()
FROM settlements s
ON CONFLICT (settlement_id, good_key) DO UPDATE SET
    amount  = settlement_goods.amount
                + GREATEST(0, EXCLUDED.amount),
    rate    = settlement_goods.rate + EXCLUDED.rate,
    cap     = GREATEST(settlement_goods.cap, EXCLUDED.cap),
    calc_at = now();

-- ── Migrate stone column → stone in settlement_goods ───────────────────────
INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_at)
SELECT
    s.id,
    'stone',
    GREATEST(0, s.stone_amount + EXTRACT(EPOCH FROM (now() - s.stone_calc_at)) / 60 * s.stone_rate),
    s.stone_rate,
    GREATEST(COALESCE(s.stone_cap, 1000), 1000),
    now()
FROM settlements s
ON CONFLICT (settlement_id, good_key) DO UPDATE SET
    amount  = settlement_goods.amount
                + GREATEST(0, EXCLUDED.amount),
    rate    = settlement_goods.rate + EXCLUDED.rate,
    cap     = GREATEST(settlement_goods.cap, EXCLUDED.cap),
    calc_at = now();

-- ── Drop food / lumber / stone columns from settlements ────────────────────
ALTER TABLE settlements
    DROP COLUMN IF EXISTS food_amount,
    DROP COLUMN IF EXISTS food_rate,
    DROP COLUMN IF EXISTS food_cap,
    DROP COLUMN IF EXISTS food_calc_at,
    DROP COLUMN IF EXISTS lumber_amount,
    DROP COLUMN IF EXISTS lumber_rate,
    DROP COLUMN IF EXISTS lumber_cap,
    DROP COLUMN IF EXISTS lumber_calc_at,
    DROP COLUMN IF EXISTS stone_amount,
    DROP COLUMN IF EXISTS stone_rate,
    DROP COLUMN IF EXISTS stone_cap,
    DROP COLUMN IF EXISTS stone_calc_at;
