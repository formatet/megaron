-- Reverse migration 073.
DROP INDEX IF EXISTS idx_events_stream_tick;
ALTER TABLE events DROP COLUMN world_tick;
