-- Migration 033: timber abundance
--
-- Timber is designed as the "local, abundant building/fuel wood" (see 031), yet
-- at the 031/032 rates it became the single biggest live bottleneck: ~1300
-- "build barracks: insufficient resources (timber)" failures in the playtest
-- logs, and settlements that lost their starting garrison hit a hard circular
-- deadlock — a barracks (80 timber) is needed to recruit infantry, infantry to
-- outpost a forest, and a forest outpost to produce timber. The 032 trickle was
-- meant to guarantee recovery but at 0.02/min (~29/day) a bricked coastal
-- settlement needed ~3 days to afford a barracks, so the guarantee was hollow.
--
-- This raises all three timber rates so timber behaves as the intended abundant
-- bulk material and the universal trickle is a real anti-deadlock floor
-- (recover a lumbermill in hours, not days). Cedar stays scarce/deposit-gated,
-- so naval/siege demand still drives trade. Costs are unchanged — supply only.
--
--   universal trickle  0.02 → 0.10  (~144/day, any terrain, no building)
--   forest baseline    0.06 → 0.15
--   lumbermill         0.10 → 0.25

UPDATE production_rules SET rate_per_min = 0.10
    WHERE good_key = 'timber' AND terrain_type IS NULL AND building_type IS NULL;
UPDATE production_rules SET rate_per_min = 0.15
    WHERE good_key = 'timber' AND terrain_type = 'forest_olive_grove' AND building_type IS NULL;
UPDATE production_rules SET rate_per_min = 0.25
    WHERE good_key = 'timber' AND building_type = 'lumbermill';

-- Recompute every active settlement's timber rate from the new rules. Accrue
-- pending production at the old rate first so no timber is lost, then set the
-- new summed rate (terrain baseline + trickle + any lumbermill).
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
