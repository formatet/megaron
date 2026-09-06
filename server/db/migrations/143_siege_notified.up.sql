-- Migration 143: belägringens start-/lyft-dispatch (megaron_plan_belagringsdispatch.md,
-- Timothy 2026-09-06). settlements.besieged is written by RecomputeProduction
-- itself (recompute.go ~line 622) BEFORE economy.SyncSiegeState runs each
-- tick, so besieged can never be compared against its own previous tick's
-- value the way SyncHexBlockade compares against settlement_placement.blockaded
-- (mig 142) — that flag is owned by the syncer, this one is not. This column
-- is SyncSiegeState's own memory of "have I already dispatched for the
-- current besieged state", untouched by RecomputeProduction.
--
-- Backfill: settlements already besieged at deploy time get siege_notified
-- set to their CURRENT besieged value, so the first sync run after this
-- migration sees no transition and does not fire a spurious SiegeStarted for
-- a siege that has been running for days.
ALTER TABLE settlements ADD COLUMN siege_notified BOOLEAN NOT NULL DEFAULT false;
UPDATE settlements SET siege_notified = besieged;
