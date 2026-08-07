DROP INDEX IF EXISTS idx_settlements_occupied;
ALTER TABLE settlements
    DROP COLUMN occupant_id,
    DROP COLUMN occupied_since_tick,
    DROP COLUMN annex_ready_notified,
    DROP COLUMN recolonizable_after_tick;
