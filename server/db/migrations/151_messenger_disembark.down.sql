ALTER TABLE messengers
    DROP COLUMN IF EXISTS disembark_at,
    DROP COLUMN IF EXISTS boarded_at,
    DROP COLUMN IF EXISTS disembark_r,
    DROP COLUMN IF EXISTS disembark_q;
