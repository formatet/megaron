-- Revert migration 033: restore 031/032 timber rates and recompute settlement rates.

UPDATE production_rules SET rate_per_min = 0.02
    WHERE good_key = 'timber' AND terrain_type IS NULL AND building_type IS NULL;
UPDATE production_rules SET rate_per_min = 0.06
    WHERE good_key = 'timber' AND terrain_type = 'forest_olive_grove' AND building_type IS NULL;
UPDATE production_rules SET rate_per_min = 0.10
    WHERE good_key = 'timber' AND building_type = 'lumbermill';

UPDATE settlement_goods sg SET
    amount  = LEAST(sg.cap,
                  sg.amount + EXTRACT(EPOCH FROM (now() - sg.calc_at))/60 * sg.rate),
    rate    = COALESCE(r.rate, 0),
    calc_at = now()
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
WHERE sg.settlement_id = s.id
  AND s.state != 'sunk'
  AND sg.good_key = 'timber';
