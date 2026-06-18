-- Ensure every settlement has a silver good row (cap 1000, matching the old silver_cap).
INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_at)
SELECT s.id, 'silver', 0, 0, 1000, now()
FROM settlements s
ON CONFLICT (settlement_id, good_key) DO NOTHING;

-- Fold the column representation into the row (settle BOTH sides first).
UPDATE settlement_goods sg
SET amount = LEAST(sg.cap,
        settled(sg.amount, sg.rate, sg.calc_at)
        + GREATEST(0, settled(s.silver_amount, s.silver_rate, s.silver_calc_at))),
    calc_at = now()
FROM settlements s
WHERE sg.settlement_id = s.id AND sg.good_key = 'silver';

-- Silver now lives only in settlement_goods; drop the kran-fed columns.
ALTER TABLE settlements
    DROP COLUMN silver_amount,
    DROP COLUMN silver_rate,
    DROP COLUMN silver_cap,
    DROP COLUMN silver_calc_at;
