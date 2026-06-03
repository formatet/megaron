-- Revert migration 032: universal timber trickle
UPDATE settlement_goods sg SET
    amount  = LEAST(sg.cap,
                  sg.amount + EXTRACT(EPOCH FROM (now() - sg.calc_at))/60 * sg.rate),
    rate    = GREATEST(0, sg.rate - 0.02),
    calc_at = now()
FROM settlements s
WHERE sg.settlement_id = s.id
  AND s.state != 'sunk'
  AND sg.good_key = 'timber';

DELETE FROM production_rules WHERE good_key = 'timber' AND terrain_type IS NULL AND building_type IS NULL;
