-- Migration 031: timber / ädelträ split
--
-- Splits "wood" into two goods:
--   * timber — local, abundant building/fuel wood (oak/pine/olive). Buildings now
--     cost timber, so coastal/non-cedar settlements are never deadlocked on cedar.
--   * cedar  — scarce, deposit-gated luxury/strategic wood (ädelträ). Stays the
--     material for ships and catapults, so naval/siege power still drives trade.
--
-- Production: forest baseline + any lumbermill. Rules are non-overlapping per
-- building (only one lumbermill rule) to avoid the ON CONFLICT "row a second time"
-- failure that bit catchment (see join.go).

-- ── timber good ────────────────────────────────────────────────────────────
INSERT INTO goods (key, name, tier, category, base_value, weight) VALUES
    ('timber', 'Timber', 'commodity', 'bulk', 4.0, 3.0)
ON CONFLICT (key) DO NOTHING;

-- ── production rules ───────────────────────────────────────────────────────
-- Forest baseline (day 1, no building). Any lumbermill (any terrain) adds a
-- steady rate so coastal settlements become self-sufficient once they build one.
INSERT INTO production_rules (terrain_type, building_type, good_key, rate_per_min, requires_deposit) VALUES
    ('forest_olive_grove', NULL,         'timber', 0.06, NULL),
    (NULL,                 'lumbermill', 'timber', 0.10, NULL);

-- ── seed existing settlements ──────────────────────────────────────────────
-- Grant 200 timber (matches the join startpaket) so the cost-model change does
-- not brick settlements that were holding cedar to build. Rate is derived from
-- the rules above given each settlement's terrain and existing buildings.
INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_at)
SELECT s.id, 'timber', 200, COALESCE(r.rate, 0), 500, now()
FROM settlements s
JOIN provinces prov ON prov.id = s.province_id
LEFT JOIN LATERAL (
    SELECT SUM(pr.rate_per_min) AS rate
    FROM production_rules pr
    WHERE pr.good_key = 'timber'
      AND (pr.terrain_type IS NULL OR pr.terrain_type = prov.terrain_type)
      AND (pr.building_type IS NULL
           OR EXISTS (SELECT 1 FROM buildings b
                      WHERE b.settlement_id = s.id AND b.building_type = pr.building_type))
) r ON true
WHERE s.state != 'sunk'
ON CONFLICT (settlement_id, good_key) DO NOTHING;
