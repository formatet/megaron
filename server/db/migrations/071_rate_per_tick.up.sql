-- Migration 071: retire the per-minute production unit.
--
-- production_rules.rate_per_min held per-MINUTE rates (e.g. grain plains 0.03/min).
-- The clean-break tick substrate (mig 067) made settled() accrue per TICK, but a
-- tick is one game-HOUR (events.TicksPerDay = 24), so the per-minute rates were
-- 60× too low against per-day consumption (pop*0.5/day) — universal starvation.
--
-- Fix (cleanup, not a design lock): scale the rates ×60 (min → hour) and rename
-- the column to rate_per_tick so the "minute" unit disappears from the schema.
-- Everything downstream (settlement_goods.rate, kharis, upkeep) is already per-
-- tick / per-day and needs no change.
--
-- CLEAN-BREAK: a reseed is expected after this (single-world-enforcement replaces
-- the running world), but the ×60 + rename is also correct for existing rows.

UPDATE production_rules SET rate_per_min = rate_per_min * 60;

ALTER TABLE production_rules RENAME COLUMN rate_per_min TO rate_per_tick;
