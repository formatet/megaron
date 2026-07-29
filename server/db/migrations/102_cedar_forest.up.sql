-- Migration 102: cedar becomes its own terrain (S2, megaron_cederskogen_plan.md)
--
-- PROBLEM (kodverifierat i DB 2026-07-29): three production_rules rows produced
-- cedar, and only one of them was tied to the actual CedarDeposit flag:
--
--   forest_olive_grove | lumbermill | cedar | 6 | requires_deposit='cedar'   (legit)
--   forest_olive_grove | NULL       | cedar | 1.2 | NULL                     (LEAK)
--   NULL               | lumbermill | cedar | 3 | requires_deposit='cedar'   (legit)
--
-- The middle row has no deposit gate at all: every forest_olive_grove hex in a
-- catchment yielded 1.2 cedar/tick whether or not it held the deposit — and
-- since forest_olive_grove is the world's single most common forest terrain,
-- cedar was never actually scarce. cedar_deposit (2-3 clustered stands per
-- map after this migration's mapgen change) meant nothing in practice.
--
-- FIX: cedar forest (`forest_cedar`) is now its own terrain, not a flag on
-- forest_olive_grove — see internal/world/model.go TerrainForestCedar and
-- mapgen.go's new cedar-stand pass. CedarDeposit becomes a pure mirror of
-- that terrain (Terrain == forest_cedar ⟺ CedarDeposit == true), set in
-- exactly one place (mapgen.go's tile-build loop) — so a second, driftable
-- source of truth never exists. requires_deposit is NULL on the new rules
-- ON PURPOSE: the terrain itself is the gate now (temenos_terrangrendering.md
-- princip 40's "two gates for the same thing can drift apart" applies here
-- identically to how it applied there).
--
-- All three old rows are removed — the legit two would become permanently
-- dead anyway (no forest_olive_grove tile will ever have cedar_deposit=true
-- again), so removing them outright is cleanup, not just leak-plugging.
--
-- CALIBRATION (A3 gate, megaron_cederskogen_plan.md): a forest_cedar hex with
-- a built lumbermill now yields 3.0 + 6.0 = 9.0 cedar/tick — the same total
-- the legit (deposit-gated) rules gave a cedar-deposit olive grove with a
-- lumbermill before this migration (6 + 3 = 9). Same order of magnitude, by
-- design, per the plan's A3 gate — see the process report for the live
-- acceptance-world DB comparison.
--
-- Timber: forest_cedar also gets a field timber rule (a cedar forest is still
-- a forest), deliberately held UNDER forest_olive_grove's 9/tick baseline —
-- cedar forest should be valuable for CEDAR, not the best timber source.
--
-- NO BACKFILL (same reasoning as migration 092's NOTE ON SCOPE): kharis/
-- tick.go calls economy.RecomputeProduction every tick for every active
-- settlement, and that function reads production_rules live. Existing
-- settlements pick the new rules up within one tick — INTENTIONALLY: cedar
-- income in a live world DROPS the moment this migration runs, for any
-- settlement whose catchment lacks a forest_cedar hex. That drop is the
-- entire point of the migration, not a bug to soften.

DELETE FROM production_rules
 WHERE good_key = 'cedar'
   AND (terrain_type = 'forest_olive_grove'
        OR (terrain_type IS NULL AND building_type = 'lumbermill'));

INSERT INTO production_rules (terrain_type, building_type, good_key, rate_per_tick, requires_deposit) VALUES
    ('forest_cedar', NULL,         'cedar',  3.0, NULL),
    ('forest_cedar', 'lumbermill', 'cedar',  6.0, NULL),
    ('forest_cedar', NULL,         'timber', 6.0, NULL);
