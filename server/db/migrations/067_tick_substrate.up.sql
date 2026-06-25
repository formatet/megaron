-- Migration 067: clean-break tick substrate.
-- Renames the three "lazy-eval anchor" columns from timestamptz to integer tick counters,
-- redefines settled() to use the world tick instead of wall-clock time, and introduces
-- current_world_tick() as the global tick lookup.
--
-- IMPORTANT: this is a CLEAN-BREAK migration. A reseed is required after applying it.
-- All existing calc_at values are set to 0 (tick origin), which is safe because
-- single-world-enforcement means the running world will be replaced by a fresh reseed
-- before these rows are read with the new formula.

-- ── Confirmation check (verified before writing this migration) ──────────────────
-- Columns renamed:
--   settlement_goods.calc_at           TIMESTAMPTZ  (mig 008)  → calc_tick INT
--   player_world_records.kharis_calc_at TIMESTAMPTZ  (mig 029)  → kharis_calc_tick INT
--   kingdoms.silver_calc_at             TIMESTAMPTZ  (mig 041)  → silver_calc_tick INT
-- settlements.silver_calc_at was DROPPED in mig 057 (silver unified into settlement_goods).

-- ── Global tick lookup ────────────────────────────────────────────────────────────
-- current_world_tick(): returns the active world's current tick.
-- Single-world-enforcement (mig 063/064) guarantees ≤1 active world at a time.
CREATE OR REPLACE FUNCTION current_world_tick() RETURNS int LANGUAGE sql STABLE AS $$
    SELECT current_tick FROM worlds WHERE status = 'active' LIMIT 1
$$;

-- ── settlement_goods.calc_at → calc_tick INT ────────────────────────────────────
ALTER TABLE settlement_goods
    ALTER COLUMN calc_at DROP DEFAULT,
    ALTER COLUMN calc_at TYPE int USING 0,
    ALTER COLUMN calc_at SET DEFAULT 0;
ALTER TABLE settlement_goods RENAME COLUMN calc_at TO calc_tick;

-- ── player_world_records.kharis_calc_at → kharis_calc_tick INT ─────────────────
ALTER TABLE player_world_records
    ALTER COLUMN kharis_calc_at DROP DEFAULT,
    ALTER COLUMN kharis_calc_at TYPE int USING 0,
    ALTER COLUMN kharis_calc_at SET DEFAULT 0;
ALTER TABLE player_world_records RENAME COLUMN kharis_calc_at TO kharis_calc_tick;

-- ── kingdoms.silver_calc_at → silver_calc_tick INT ─────────────────────────────
ALTER TABLE kingdoms
    ALTER COLUMN silver_calc_at DROP DEFAULT,
    ALTER COLUMN silver_calc_at TYPE int USING 0,
    ALTER COLUMN silver_calc_at SET DEFAULT 0;
ALTER TABLE kingdoms RENAME COLUMN silver_calc_at TO silver_calc_tick;

-- ── Redefine settled() to use integer ticks ─────────────────────────────────────
-- Drop the old timestamptz-based version first.
DROP FUNCTION IF EXISTS settled(double precision, double precision, timestamptz);

-- New tick-based formula: amount + rate_per_tick × GREATEST(0, current_tick − calc_tick).
-- Uses current_world_tick() as the global tick source — no arg change at 80 call sites.
CREATE FUNCTION settled(p_amount double precision, p_rate double precision, p_calc_tick int)
RETURNS double precision LANGUAGE sql STABLE AS $$
    SELECT p_amount + p_rate * GREATEST(0, current_world_tick() - p_calc_tick)
$$;

-- ── Drop sim_config (wall-clock substrate artifact) ─────────────────────────────
DROP TABLE IF EXISTS sim_config;
