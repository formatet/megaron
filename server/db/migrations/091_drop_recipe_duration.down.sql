-- Rollback: restore the column (original mig 010 shape was NOT NULL; the seed
-- values are not reconstructed — a default keeps existing rows valid).
ALTER TABLE recipes ADD COLUMN IF NOT EXISTS duration_min FLOAT NOT NULL DEFAULT 0;
