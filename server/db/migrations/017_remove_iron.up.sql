-- Iron is anachronistic in a Bronze Age setting.
-- Drop iron resource columns from settlements; Mine building repurposed to yield stone.
ALTER TABLE settlements
    DROP COLUMN IF EXISTS iron_amount,
    DROP COLUMN IF EXISTS iron_rate,
    DROP COLUMN IF EXISTS iron_cap,
    DROP COLUMN IF EXISTS iron_calc_at;
