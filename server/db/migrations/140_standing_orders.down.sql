ALTER TABLE transports DROP COLUMN IF EXISTS standing_order_id;
DROP TABLE IF EXISTS standing_order_return_goods;
DROP TABLE IF EXISTS standing_order_outbound_goods;
DROP TABLE IF EXISTS standing_orders;
