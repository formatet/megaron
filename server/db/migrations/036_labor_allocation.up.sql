-- Migration 036: labor allocation
--
-- Introduces population-driven production (variant B, Steg 1).
--
-- New table `settlement_labor` holds per-good allocation weights (sliders,
-- Σ=1 over producible goods). The RecomputeProduction helper in
-- internal/economy uses these weights together with labor_pool to set
-- settlement_goods.rate for each producible good.
--
-- population semantics: population is now the demographic total (incl. soldiers
-- as individuals). Recruit no longer decrements population; combat casualties
-- decrement it. The labor_pool formula:
--   labor_pool = max(0, population − Σ(army_cols × PopCost) − Σ(in_transit × PopCost))
--
-- The unconditional timber trickle (migration 032/033, 0.10/min) is preserved
-- as a labor-free baseline so the barracks deadlock cannot recur.

CREATE TABLE IF NOT EXISTS settlement_labor (
    settlement_id UUID    NOT NULL REFERENCES settlements(id) ON DELETE CASCADE,
    good_key      TEXT    NOT NULL REFERENCES goods(key),
    weight        DOUBLE PRECISION NOT NULL DEFAULT 0,
    PRIMARY KEY (settlement_id, good_key)
);

-- Seed uniform weights for all existing settlements over their producible goods.
-- "Producible" = good_key that has at least one matching production_rule given
-- the settlement's terrain, deposits, and completed buildings.
INSERT INTO settlement_labor (settlement_id, good_key, weight)
SELECT
    s.id                        AS settlement_id,
    agg.good_key,
    1.0 / COUNT(*) OVER (PARTITION BY s.id) AS weight
FROM settlements s
JOIN provinces prov ON prov.id = s.province_id
-- Gather all good_keys this settlement can produce
JOIN LATERAL (
    SELECT DISTINCT pr.good_key
    FROM production_rules pr
    WHERE
        (pr.terrain_type IS NULL OR pr.terrain_type = prov.terrain_type)
        AND (pr.building_type IS NULL OR EXISTS (
                SELECT 1 FROM buildings b
                WHERE b.settlement_id = s.id AND b.building_type = pr.building_type))
        AND (pr.requires_deposit IS NULL
             OR (pr.requires_deposit = 'copper' AND prov.copper_deposit)
             OR (pr.requires_deposit = 'tin'    AND prov.tin_deposit)
             OR (pr.requires_deposit = 'silver' AND COALESCE(prov.silver_deposit, false))
             OR (pr.requires_deposit = 'cedar'  AND COALESCE(prov.cedar_deposit, false)))
) agg ON true
WHERE s.state != 'sunk'
ON CONFLICT (settlement_id, good_key) DO NOTHING;
