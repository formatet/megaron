-- Reverse migration 071: restore the per-minute column name and magnitude.
ALTER TABLE production_rules RENAME COLUMN rate_per_tick TO rate_per_min;

UPDATE production_rules SET rate_per_min = rate_per_min / 60;
