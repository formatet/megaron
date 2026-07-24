-- Revert migration 097: Sitos tick idempotency marker
DROP TABLE IF EXISTS processed_sitos_ticks;
