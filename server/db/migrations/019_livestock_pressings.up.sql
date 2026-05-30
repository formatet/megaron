-- Migration 019: livestock good + olive_press + winery buildings

-- ── New goods ─────────────────────────────────────────────────────────────────
INSERT INTO goods (key, name, tier, category, base_value, weight) VALUES
    ('livestock', 'Livestock', 'commodity', 'staple', 5.0, 3.0);

-- ── Production rules ──────────────────────────────────────────────────────────
INSERT INTO production_rules (terrain_type, building_type, good_key, rate_per_min, requires_deposit) VALUES
    -- livestock: plains terrain baseline (cattle, sheep, goats graze freely)
    ('plains',   NULL,          'livestock', 0.025, NULL),
    -- olive press boosts oil production significantly
    ('hills',    'olive_press', 'oil',       0.04,  NULL),
    ('plains',   'olive_press', 'oil',       0.03,  NULL),
    -- winery boosts wine production significantly
    ('hills',    'winery',      'wine',      0.05,  NULL);

-- ── Seed livestock in existing plains settlements ─────────────────────────────
INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_at)
SELECT s.id, 'livestock', 0, 0.025, 200, now()
FROM settlements s
JOIN provinces p ON p.id = s.province_id
WHERE p.terrain_type = 'plains'
  AND s.state != 'sunk'
ON CONFLICT (settlement_id, good_key) DO NOTHING;
