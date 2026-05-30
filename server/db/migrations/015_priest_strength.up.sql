-- Priest strength pool: gates how many libations/rites a settlement can perform.
-- Regenerates daily based on stationed priests. Capped at 100.
ALTER TABLE settlements ADD COLUMN priest_strength SMALLINT NOT NULL DEFAULT 100;
