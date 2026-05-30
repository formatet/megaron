-- Migration 013: invasions_today counter for scaling defensive casualties.
-- Reset to 0 at each daily kharis tick. Prevents chain-invasion zero-out.
ALTER TABLE settlements ADD COLUMN IF NOT EXISTS invasions_today INT NOT NULL DEFAULT 0;
