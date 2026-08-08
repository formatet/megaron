-- Migration 120: settlement_placement (gubbemodell P4, megaron_plan_fysisk_gubbemodell.md).
--
-- Replaces settlement_labor's per-VARA weight (a percentage of the whole city)
-- with an individually addressable gubbe on a PLATS: one catchment hex, or one
-- building slot. Production is now derived from where placed gubbar actually
-- stand (economy.RecomputeProduction), not from an aggregate weight applied to
-- the whole labor pool. settlement_labor is NOT dropped — cult labor (temple
-- devotion, good_key='cult') is an explicitly separate path
-- (megaron_cult_ar_ingen_vara_plan.md) and keeps reading/writing it unchanged.
--
-- gubbe_ordinal is a stable 1..floor(population/100) identity within a
-- settlement (Temenos_varutaxonomi_sol.md §1.1: "1 gubbe = 100 invånare") — the
-- population remainder (pop mod 100) is NOT a placeable gubbe; it auto-farms
-- (economy.RecomputeProduction's remainder term), unchanged by this table.
--
-- target_kind splits a placement into exactly one of two shapes, enforced by
-- the CHECK below: 'hex' (a catchment ring hex, hex_q/hex_r set, the worker's
-- good_key is the role picked for that hex — a plains hex can hold a grain
-- worker OR a livestock worker, never both at once from the SAME row) or
-- 'building' (a city workplace slot, building_type set, no coordinates — the
-- building sits on the settlement's own hex, not a ring hex). No live good
-- routes through 'building' yet at P4 (foundry=P6 recipe path, market/stable
-- goods are parked) — the shape exists now so P5/P6 don't need a second
-- migration.
CREATE TABLE settlement_placement (
    id            BIGSERIAL PRIMARY KEY,
    settlement_id UUID NOT NULL REFERENCES settlements(id) ON DELETE CASCADE,
    gubbe_ordinal INT NOT NULL CHECK (gubbe_ordinal >= 1),
    target_kind   TEXT NOT NULL CHECK (target_kind IN ('hex', 'building')),
    hex_q         INT,
    hex_r         INT,
    building_type TEXT,
    good_key      TEXT NOT NULL REFERENCES goods(key),
    placed_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (settlement_id, gubbe_ordinal),
    CHECK (
        (target_kind = 'hex'      AND hex_q IS NOT NULL AND hex_r IS NOT NULL AND building_type IS NULL)
        OR
        (target_kind = 'building' AND building_type IS NOT NULL AND hex_q IS NULL AND hex_r IS NULL)
    )
);

-- RecomputeProduction's hot path: "who is standing on/in this target doing
-- this good" — grouped by (settlement, hex, good) or (settlement, building, good).
CREATE INDEX ON settlement_placement (settlement_id, hex_q, hex_r, good_key)
    WHERE target_kind = 'hex';
CREATE INDEX ON settlement_placement (settlement_id, building_type, good_key)
    WHERE target_kind = 'building';
