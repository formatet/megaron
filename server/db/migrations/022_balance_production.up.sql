-- Migration 022: Production balance
-- Farm grain and lumbermill cedar rates were set to 2.0/min (2880/day) in migration 018,
-- which caused grain/cedar to always be at cap → no scarcity → no trade pressure.
-- New rates: 0.1/min (144/day), matching terrain-only baselines in scale.
-- Also fixes existing settlement_goods rows for farms/lumbermills already built.

-- Fix future buildings
UPDATE production_rules SET rate_per_min = 0.1
    WHERE building_type = 'farm' AND good_key = 'grain';

UPDATE production_rules SET rate_per_min = 0.1
    WHERE building_type = 'lumbermill' AND good_key = 'cedar';

-- Settle current amount (apply lazy-eval interest), then subtract the old bonus (2.0-0.1=1.9/min).
-- Only settlements with a farm building got the inflated grain rate.
UPDATE settlement_goods sg SET
    amount  = GREATEST(0, LEAST(sg.cap,
                  sg.amount + EXTRACT(EPOCH FROM (now() - sg.calc_at))/60 * sg.rate)),
    rate    = GREATEST(0, sg.rate - 1.9),
    calc_at = now()
FROM buildings b
WHERE sg.settlement_id = b.settlement_id
  AND b.building_type = 'farm'
  AND sg.good_key = 'grain';

UPDATE settlement_goods sg SET
    amount  = GREATEST(0, LEAST(sg.cap,
                  sg.amount + EXTRACT(EPOCH FROM (now() - sg.calc_at))/60 * sg.rate)),
    rate    = GREATEST(0, sg.rate - 1.9),
    calc_at = now()
FROM buildings b
WHERE sg.settlement_id = b.settlement_id
  AND b.building_type = 'lumbermill'
  AND sg.good_key = 'cedar';
