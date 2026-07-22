-- Migration 092: the olive grove produces olives (Timothy 2026-07-22)
--
-- forest_olive_grove had exactly ONE production rule — timber (031, raised in
-- 033) — so the terrain that is literally named "olive grove" yielded no oil at
-- all. Oil came only from plains (1.2 baseline) and hills (0.9), and olive_press
-- had no rule on the grove either: a Wanax who saw an olive grove in the
-- catchment, built a press, and got nothing back was reading the map correctly
-- and being punished for it. The name promised a production chain that did not
-- exist.
--
-- The grove now leads on oil, as its name says. Rates are additive per hex
-- (catchment.go / recompute.go both SUM the matching rules), giving:
--
--   grove   1.8 baseline  + 3.0 olive_press = 4.8   ← best olive land, one building
--   plains  1.2 baseline  + 1.8 farm + 1.8 press = 4.8  (same ceiling, two buildings)
--   hills   0.9 baseline  + 2.4 press = 3.3
--
-- So the grove is the efficient olive land and plains stays the versatile one
-- (grain AND oil). The baseline sits above plains deliberately: a grove is
-- already an orchard, olives grow there untended. Magnitude is calibrated
-- against the sink that matters — a temple burns 2.0 oil/tick
-- (kharis.OfferOilPerTemple) — so one grove hex nearly feeds one temple. These
-- are tunables, not invariants.
--
-- NOTE ON SCOPE: forest_olive_grove is the world's ONLY forest terrain (028
-- renamed `forest` wholesale), so this makes every forest hex yield oil. That
-- double duty is a pre-existing simplification of the terrain enum, not
-- something this migration introduces — see megaron_todo "SB6 terräng-enum-
-- reconciliering" if the two ever want splitting.
--
-- No backfill needed (unlike 033, which predates the catchment model): the pop
-- tick calls economy.RecomputeProduction for every active settlement every tick
-- (kharis/tick.go), and that reads production_rules live — so existing
-- settlements pick the new rates up within one tick.

INSERT INTO production_rules (terrain_type, building_type, good_key, rate_per_tick, requires_deposit) VALUES
    ('forest_olive_grove', NULL,          'oil', 1.8, NULL),
    ('forest_olive_grove', 'olive_press', 'oil', 3.0, NULL);
