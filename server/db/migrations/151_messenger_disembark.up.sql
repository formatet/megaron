-- Migration 151: kartans rendering av budets tre ben (megaron_plan_budets_tre_ben.md,
-- slice 3c).
--
-- boardOne already computes disembarkAt (the hex a boarded messenger steps
-- ashore at) and carrierArrivesAt (when that happens) but only ever stored
-- their SUM (arrives_at, the final delivery time) — the moment of boarding
-- itself and the disembark point/time were thrown away, so the landward leg
-- after stepping off the carrier could never be drawn (same stop condition
-- 3b-1 wrote into province/eyes.go). All four columns are NULL for a
-- messenger that never needed the sea, or that boarded before this migration
-- shipped (R3's own NULL-columns leg falls back to a stationary port marker
-- rather than guessing).
--
-- disembark_q/r: the hex the carrier's OWN destination resolves to (the same
-- disembarkAt boardOne already had in hand).
-- boarded_at: the moment this boarding happened (h.clk.Now() at boardOne).
-- disembark_at: when the carrier actually reaches disembarkAt — this IS the
-- carrierArrivesAt boardOne already adds landDur to, to get arrives_at.
ALTER TABLE messengers
    ADD COLUMN disembark_q INT,
    ADD COLUMN disembark_r INT,
    ADD COLUMN boarded_at TIMESTAMPTZ,
    ADD COLUMN disembark_at TIMESTAMPTZ;
