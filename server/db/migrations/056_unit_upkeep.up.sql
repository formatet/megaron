-- Migration 056: W4e — armé-upkeep. unpaid_periods räknar obetalda silver-perioder
-- (desertering vid tröskel). Upkeep-rater bor som Go-map (KISS, likt UnitSpecs).
ALTER TABLE units ADD COLUMN IF NOT EXISTS unpaid_periods INT NOT NULL DEFAULT 0;
