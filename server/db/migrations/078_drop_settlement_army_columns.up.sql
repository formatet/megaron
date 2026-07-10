-- SB7 / Fas D — retire the frozen integer army columns on settlements.
--
-- These columns were the pre-C2 army representation. Since the C1–C8 unit model
-- (mig 047) the units table is the single source of truth for a settlement's
-- standing army: combat (internal/combat/unit_arrival.go), loyalty decay, recruit
-- and starter units all read/write `units WHERE status='garrison'`. The columns
-- below were only kept alive by display reads (db.go, god.go) and a few tick
-- writers (starvation/divine events) — a dual-write era that had drifted into
-- pure divergence (a settlement's shown army no longer matched what fought).
--
-- Every remaining reader/writer was moved to the units table in the same change
-- that ships this migration; the columns are now unreferenced. Drop them.
--
-- No data fold: the columns hold stale residue (seed-era dual-writes + accumulated
-- divine recruits that never fought). Folding it into units would resurrect phantom
-- armies. The live army already lives in units — nothing real is lost.
ALTER TABLE settlements
    DROP COLUMN IF EXISTS infantry,
    DROP COLUMN IF EXISTS chariot,
    DROP COLUMN IF EXISTS priest,
    DROP COLUMN IF EXISTS ship,
    DROP COLUMN IF EXISTS elite_infantry,
    DROP COLUMN IF EXISTS war_galley,
    DROP COLUMN IF EXISTS merchantman;
