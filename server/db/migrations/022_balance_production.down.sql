-- Reverse migration 022: restore inflated rates
UPDATE production_rules SET rate_per_min = 2.0
    WHERE building_type = 'farm' AND good_key = 'grain';

UPDATE production_rules SET rate_per_min = 2.0
    WHERE building_type = 'lumbermill' AND good_key = 'cedar';

-- Re-add the 1.9/min bonus to settlements that have the buildings.
-- (approximate reverse — amounts have moved since migration ran)
UPDATE settlement_goods sg SET
    amount  = GREATEST(0, LEAST(sg.cap,
                  sg.amount + EXTRACT(EPOCH FROM (now() - sg.calc_at))/60 * sg.rate)),
    rate    = sg.rate + 1.9,
    calc_at = now()
FROM buildings b
WHERE sg.settlement_id = b.settlement_id
  AND b.building_type = 'farm'
  AND sg.good_key = 'grain';

UPDATE settlement_goods sg SET
    amount  = GREATEST(0, LEAST(sg.cap,
                  sg.amount + EXTRACT(EPOCH FROM (now() - sg.calc_at))/60 * sg.rate)),
    rate    = sg.rate + 1.9,
    calc_at = now()
FROM buildings b
WHERE sg.settlement_id = b.settlement_id
  AND b.building_type = 'lumbermill'
  AND sg.good_key = 'cedar';
