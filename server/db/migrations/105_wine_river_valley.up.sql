-- Migration 105: vin i floddalen
-- (Timothy 2026-08-02: river_valley ska bära vin på slättens nivå, inte
-- hills nivå och inte scrub_maquis nivå — floddalen är bördigare mark än
-- den marginella snåren men inte den priviligierade sluttningen.)
--
-- Migration 103 gav vin till plains och scrub_maquis men lämnade
-- river_valley medvetet orörd ("Timothy har inte uttalat sig om flodmark
-- som vinmark" — 103:s eget kommentarhuvud). Timothy har nu uttalat sig:
-- river_valley får samma tre rader som plains.
--
--   hills          1.2 baslinje · 2.4 med farm · 3.0 med winery   (OFÖRÄNDRAT)
--   plains         0.6 baslinje · 1.2 med farm · 1.8 med winery   (OFÖRÄNDRAT, mig 103)
--   river_valley   0.6 baslinje · 1.2 med farm · 1.8 med winery   (nytt, samma som plains)
--   scrub_maquis   0.4 baslinje ·  (ingen farm) · 1.0 med winery  (OFÖRÄNDRAT, mig 103)
--
-- river och river_delta får INGET vin här — bara river_valley. hills, plains
-- och scrub_maquis rörs inte av den här migrationen.
--
-- requires_deposit är NULL på alla tre raderna med avsikt: det här är inget
-- deposit-gatat mineral, det är en gröda som växer där jorden tillåter det.
--
-- NO BACKFILL (samma resonemang som migration 092/102/103): kharis/tick.go
-- och alla andra call sites till economy.RecomputeProduction läser
-- production_rules levande varje gång de körs. Befintliga städer plockar upp
-- den nya raden inom en tick av nästa recompute — ingen backfill-loop behövs
-- eller är korrekt (staden kan ha bytt catchment sedan seed).
--
-- SCOPE: den här migrationen rör bara vin, bara river_valley, tre rader.
-- Ingen annan vara och inte goods.base_value rebalanseras här. Ingen ändring
-- på river eller river_delta.

INSERT INTO production_rules (terrain_type, building_type, good_key, rate_per_tick, requires_deposit) VALUES
    ('river_valley', NULL,     'wine', 0.6, NULL),
    ('river_valley', 'farm',   'wine', 1.2, NULL),
    ('river_valley', 'winery', 'wine', 1.8, NULL);
