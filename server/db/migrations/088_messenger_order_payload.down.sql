DELETE FROM messengers WHERE order_payload IS NOT NULL;
ALTER TABLE messengers DROP COLUMN order_payload;
