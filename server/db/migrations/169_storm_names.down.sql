DROP INDEX IF EXISTS idx_sea_storms_name;
ALTER TABLE sea_storms DROP COLUMN IF EXISTS named_tick, DROP COLUMN IF EXISTS claimed_tick,
    DROP COLUMN IF EXISTS claimed_by, DROP COLUMN IF EXISTS name;
