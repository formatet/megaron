-- Loyalty becomes a slow continuous accumulator (kharis-model, Timothy 2026-07-11):
-- a hidden loyalty_points (1–100) crawls, and the integer loyalty (1–4) is DERIVED
-- from band thresholds (<25→1, 25–50→2, 50–75→3, ≥75→4). Loyalty should now be
-- built over weeks, like kharis — no single day-tick can jump a settlement to max.
--
-- The band ceilings + start values live as tunable levers in internal/loyalty/points.go
-- (documented in temenos_balans_spakar.md §12). The midpoints below mirror those bands;
-- if the Go band constants change, this one-time backfill is historical and need not.
ALTER TABLE settlements
  ADD COLUMN loyalty_points DOUBLE PRECISION NOT NULL DEFAULT 37; -- band-2 midpoint = loyalty 2

-- Preserve each existing settlement's live loyalty: seed points at its current band's
-- midpoint so nothing snaps up or down on deploy.
UPDATE settlements SET loyalty_points = CASE loyalty
    WHEN 1 THEN 12
    WHEN 2 THEN 37
    WHEN 3 THEN 62
    WHEN 4 THEN 87
    ELSE 37
END;
