ALTER TABLE units
    DROP COLUMN IF EXISTS pickup_wait_ticks,
    DROP COLUMN IF EXISTS pickup_unit_id;
