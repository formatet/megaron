DROP INDEX IF EXISTS idx_standing_orders_ship_unit;
DROP INDEX IF EXISTS idx_transports_ship_unit;
ALTER TABLE standing_orders DROP COLUMN ship_unit_id;
ALTER TABLE transports DROP COLUMN ship_unit_id;
