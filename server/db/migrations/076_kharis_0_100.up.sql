-- Migration 075: kharis rescale 0-2000 → 0-100 (Timothy 2026-07-09,
-- temenos_kharis.md §"KANONISK OMDESIGN", plan megaron_kharis_plan.md FAS 0).
--
-- Roten till omdesignen: kharis satt pegged vid kharis_cap=2000 in production —
-- the daily cult→kharis gain (~16/day) was tiny against that cap, and once a
-- Wanax reached it the LEAST(cap,...) clamp silently discarded further gains.
-- The new scale is one hidden 0-100 number with all thresholds rescaled ÷20 in
-- Go (internal/kharis/tick.go, internal/religion/prayers.go,
-- internal/combat/fortune.go). This migration rescales the DB-side data +
-- default to match. All numbers are STRAWMAN (temenos_balans_spakar.md §9).

ALTER TABLE player_world_records ALTER COLUMN kharis_cap SET DEFAULT 100;

UPDATE player_world_records SET
    kharis_amount = LEAST(100, kharis_amount / 20.0),
    kharis_cap    = 100;
