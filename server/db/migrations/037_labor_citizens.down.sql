-- Migration 037 rollback: återställ weight-kolumnen.
ALTER TABLE settlement_labor ADD COLUMN IF NOT EXISTS weight DOUBLE PRECISION NOT NULL DEFAULT 0;

-- Återskapa approximativa vikter: weight = citizens / labor_pool (eller uniform om okänd).
UPDATE settlement_labor sl
SET weight = CASE
    WHEN (
        SELECT GREATEST(1,
            s2.population
            - (s2.infantry * 5 + s2.cavalry * 8 + s2.catapult * 2
               + s2.priest * 3 + s2.ship * 10 + s2.elite_infantry * 10)
        )
        FROM settlements s2 WHERE s2.id = sl.settlement_id
    ) > 0
    THEN sl.citizens::double precision / (
        SELECT GREATEST(1,
            s2.population
            - (s2.infantry * 5 + s2.cavalry * 8 + s2.catapult * 2
               + s2.priest * 3 + s2.ship * 10 + s2.elite_infantry * 10)
        )
        FROM settlements s2 WHERE s2.id = sl.settlement_id
    )
    ELSE 0
END;

-- Normalisera så Σweight = 1.0 per settlement.
UPDATE settlement_labor sl
SET weight = sl.weight / sub.total
FROM (
    SELECT settlement_id, SUM(weight) AS total
    FROM settlement_labor
    GROUP BY settlement_id
    HAVING SUM(weight) > 0
) sub
WHERE sl.settlement_id = sub.settlement_id;

ALTER TABLE settlement_labor DROP COLUMN IF EXISTS citizens;
