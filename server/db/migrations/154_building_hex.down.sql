-- Reverses migration 154. Cannot recover which 'mine' rows were originally
-- 'silver_mine' after the up-migration's same-type merge — beslut 5 (no
-- players, no balance consideration) accepted that asymmetry going in.

ALTER TABLE buildings DROP CONSTRAINT IF EXISTS buildings_hex_bound_check;
ALTER TABLE build_queue DROP CONSTRAINT IF EXISTS build_queue_hex_bound_check;

DROP INDEX IF EXISTS buildings_settlement_building_city;
DROP INDEX IF EXISTS buildings_settlement_building_hex;
CREATE UNIQUE INDEX IF NOT EXISTS buildings_settlement_building
    ON buildings (settlement_id, building_type);

ALTER TABLE buildings DROP COLUMN IF EXISTS hex_q;
ALTER TABLE buildings DROP COLUMN IF EXISTS hex_r;
ALTER TABLE build_queue DROP COLUMN IF EXISTS hex_q;
ALTER TABLE build_queue DROP COLUMN IF EXISTS hex_r;
