-- Any lumbermill produces cedar regardless of terrain (0.05/min fallback).
-- Forest lumbermill still gets the additional forest-specific 0.1 rule → 0.15 total.
-- Prevents deadlock when no player spawns on forest terrain.
INSERT INTO production_rules (good_key, building_type, terrain_type, rate_per_min)
SELECT 'cedar', 'lumbermill', NULL, 0.05
WHERE NOT EXISTS (
    SELECT 1 FROM production_rules
    WHERE good_key = 'cedar' AND building_type = 'lumbermill' AND terrain_type IS NULL
);

-- Backfill settlement_goods for existing settlements with a lumbermill
-- that don't already produce cedar (rate = 0 means no terrain rule fired at build time).
INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_at)
SELECT DISTINCT s.id, 'cedar', 0, 0.05, 500, now()
FROM settlements s
JOIN buildings b ON b.settlement_id = s.id AND b.building_type = 'lumbermill'
ON CONFLICT (settlement_id, good_key) DO UPDATE
    SET rate    = settlement_goods.rate + 0.05,
        calc_at = now()
WHERE settlement_goods.rate = 0;
