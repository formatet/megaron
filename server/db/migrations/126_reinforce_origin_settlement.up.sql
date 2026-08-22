-- Migration 126: kohort-rekrytering + manskaps-underhåll
-- (megaron_plan_rekryteringsmodell.md, Timothy 2026-08-19)
--
-- origin_settlement_id is a NEW, stable "home city" field for a land cohort.
-- Deliberately NOT a reuse of units.home_settlement_id (migration 074) —
-- that column is a TRANSIENT march-origin marker that unit_arrival.go
-- nulls out on explore-return (the exact fella 07-26b already hit once).
-- origin_settlement_id is set ONCE at recruit time and never changed again
-- by any endpoint (march, garrison-change, a future kingdom loan) — it is
-- the answer to "whose growth pays to reinforce this cohort", which must
-- survive everything home_settlement_id does not.
ALTER TABLE units ADD COLUMN origin_settlement_id UUID REFERENCES settlements(id) ON DELETE SET NULL;

COMMENT ON COLUMN units.origin_settlement_id IS
    'The settlement that recruited this cohort and pays to reinforce it. Set once at Recruit, never changed (not by march, garrison-change, or a future kingdom loan) — distinct from the transient home_settlement_id (mig 074), which explore already nulls out. See migration 126 / megaron_plan_rekryteringsmodell.md.';

-- Backfill best-guess for existing rows: a unit currently sitting in a
-- settlement is assumed to have originated there (we have no better history).
-- Units with no settlement_id (marching/positioned/embarked/disbanded) are
-- left NULL — not reinforceable until they return home, which is correct:
-- reinforce requires status='garrison' anyway.
UPDATE units SET origin_settlement_id = settlement_id WHERE settlement_id IS NOT NULL;

-- reinforcing marks a cohort as awaiting refill from its origin city's
-- population growth. Set by POST .../units/{id}/reinforce, cleared by the
-- tick worker once the cohort reaches 100 men OR leaves its origin garrison
-- (kharis/tick.go applyReinforcement).
ALTER TABLE units ADD COLUMN reinforcing BOOLEAN NOT NULL DEFAULT false;

COMMENT ON COLUMN units.reinforcing IS
    'True while this cohort is awaiting men from its origin city''s population growth (POST .../units/{id}/reinforce). Cleared at size=100 or when the cohort leaves origin garrison. See migration 126.';
