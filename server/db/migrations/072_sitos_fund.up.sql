-- Migration 072: Sitos-fonden (σῖτος — the "grain-watcher" ever-normal granary).
--
-- Adds a capital-bounded silver pool per settlement that acts as the automatic
-- last-resort counterparty for subsistence goods (grain, fish): it buys surplus
-- and sells shortage at a smoothed reference price, so a city always has a
-- buyer/seller even with no human offer. Full model + invariants: temenos_sitos.md.
--
-- Silver invariant: the fund only ever moves silver fund↔settlement (+ this
-- one-time genesis seed + the guarded tax leg, which is also fund↔settlement).
-- sitos_fund_silver < 0 is a bug (handler floors at GREATEST(0,…)).

ALTER TABLE settlements ADD COLUMN sitos_fund_silver float8 NOT NULL DEFAULT 0;

-- Backfill settlements that already exist in the active world (single-world-
-- enforcement ⇒ at most one active world) using the DEFAULT tunable formula:
--   seed = dailyGrainNeedInSilver × SITOS_STARTING_FUND_DAYS
--        = (population × 0.5 × grain.base_value) × 10
-- New settlements created after this migration get their seed at creation time
-- (join.go / foundColony) using the live env-configured value, so no reseed is
-- required — this only covers pre-existing rows.
UPDATE settlements s
SET sitos_fund_silver = s.population * 0.5 * g.base_value * 10
FROM goods g
WHERE g.key = 'grain'
  AND s.owner_id IS NOT NULL
  AND s.world_id IN (SELECT id FROM worlds WHERE status = 'active');
