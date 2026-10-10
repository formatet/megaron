-- The harvest blessing (rite and divine favour) becomes a yield bonus for a
-- span of ticks instead of a one-shot ×1.25 on the grain store (Timothy
-- 2026-10-10): +25 % on every food good but fish for 25 ticks. NULL = never
-- blessed. RecomputeProduction reads it every tick, so the bonus ends by itself.
ALTER TABLE settlements ADD COLUMN harvest_blessing_until_tick INT;
