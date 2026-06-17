-- Rollback 049: återgå till citizens (INT).
ALTER TABLE settlement_labor ADD COLUMN IF NOT EXISTS citizens INT NOT NULL DEFAULT 0;

UPDATE settlement_labor sl
SET citizens = GREATEST(0, ROUND(sl.weight * GREATEST(s.population, 0)))
FROM settlements s
WHERE sl.settlement_id = s.id;

ALTER TABLE settlement_labor DROP COLUMN IF EXISTS weight;
