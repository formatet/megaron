-- Any lumbermill produces cedar regardless of terrain (0.05/min fallback).
-- Forest lumbermill still gets the additional forest-specific 0.1 rule → 0.15 total.
-- Prevents deadlock when no player spawns on forest terrain.
INSERT INTO production_rules (good_key, building_type, terrain_type, rate_per_min)
VALUES ('cedar', 'lumbermill', NULL, 0.05)
ON CONFLICT (good_key, building_type, terrain_type) DO NOTHING;

-- Backfill settlement_goods for existing settlements with a lumbermill
-- that don't already have cedar rate > 0 from the forest rule.
INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_at)
SELECT DISTINCT s.id, 'cedar', 0, 0.05, 500, now()
FROM settlements s
JOIN buildings b ON b.settlement_id = s.id AND b.building_type = 'lumbermill'
WHERE NOT EXISTS (
    SELECT 1 FROM settlement_goods sg
    WHERE sg.settlement_id = s.id AND sg.good_key = 'cedar' AND sg.rate > 0
)
ON CONFLICT (settlement_id, good_key) DO UPDATE
    SET rate = settlement_goods.rate + 0.05,
        calc_at = now()
WHERE settlement_goods.rate = 0;
