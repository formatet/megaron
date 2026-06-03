-- Migration 032: universal timber trickle
--
-- Guarantees "ingen kronisk brist" for timber: every settlement gathers a small
-- baseline of deadwood/scrub (0.02/min ≈ 29/day) regardless of terrain, so a
-- settlement that spends its startpaket timber before building a lumbermill can
-- never be hard-locked — it always recovers enough to afford one.
--
-- Safe against the ON CONFLICT dup-row bug: this is a terrain-only rule
-- (building_type NULL), and join.go aggregates terrain-only rules per good_key
-- (GROUP BY) while build.go only ever selects building rules.

INSERT INTO production_rules (terrain_type, building_type, good_key, rate_per_min, requires_deposit) VALUES
    (NULL, NULL, 'timber', 0.02, NULL);

-- Backfill the trickle onto every active settlement's existing timber rate.
UPDATE settlement_goods sg SET
    amount  = LEAST(sg.cap,
                  sg.amount + EXTRACT(EPOCH FROM (now() - sg.calc_at))/60 * sg.rate),
    rate    = sg.rate + 0.02,
    calc_at = now()
FROM settlements s
WHERE sg.settlement_id = s.id
  AND s.state != 'sunk'
  AND sg.good_key = 'timber';
