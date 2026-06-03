-- Revert migration 034: delivery idempotency marker
DROP TABLE IF EXISTS processed_deliveries;
