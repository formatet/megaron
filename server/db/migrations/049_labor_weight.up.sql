-- Migration 049: återgå till weight-semantik för settlement_labor.
--
-- Migration 037 bytte weight (normaliserad bråkdel) mot citizens (INT).
-- Den bytet visade sig vara fel — absoluta citizens-tal skalas inte automatiskt
-- med populationstillväxt. Vi går tillbaka till weight-semantik:
--   rate(g) = (base_potential(g) / REF_LABOR) × weight(g) × labor_pool
--
-- weight ∈ [0.0, 1.0]; Σ weight ≤ 1.0 per settlement (resten = idle).
-- Befintliga citizens-rader konverteras: weight = citizens / max(population, 1).

ALTER TABLE settlement_labor ADD COLUMN IF NOT EXISTS weight FLOAT4 NOT NULL DEFAULT 0.0;

UPDATE settlement_labor sl
SET weight = GREATEST(0.0, LEAST(1.0,
    sl.citizens::float4 / NULLIF(s.population::float4, 0)))
FROM settlements s
WHERE sl.settlement_id = s.id;

-- Normalisera så att summan per settlement inte överstiger 1.0
-- (kan hända om citizens var överskott mot population)
UPDATE settlement_labor sl
SET weight = weight / total.sum
FROM (
    SELECT settlement_id, SUM(weight) AS sum
    FROM settlement_labor
    GROUP BY settlement_id
    HAVING SUM(weight) > 1.0
) total
WHERE sl.settlement_id = total.settlement_id;

ALTER TABLE settlement_labor DROP COLUMN IF EXISTS citizens;
