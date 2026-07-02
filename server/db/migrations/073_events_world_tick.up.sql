-- Migration 073: stamp each event with the game tick it occurred on.
--
-- The events table only had created_at (wall-clock). The per-city tick-journal
-- (temenos_sitos.md Fas 2) needs to bucket discrete events by GAME tick, and
-- wall-clock → tick is lossy across downtime catch-up ticks (exactly what the
-- mig 067 clean-break tick substrate exists to avoid). Add an integer tick
-- stamp instead.
--
-- The column DEFAULTs to current_world_tick() (mig 067's global tick lookup), so
-- every existing INSERT INTO events (...) call site fills it automatically — zero
-- Go changes to events.Store.Append. Additive + backward-compatible: it never
-- reinterprets existing events (event-versioning rule intact); old rows simply
-- carry world_tick = 0.
ALTER TABLE events ADD COLUMN world_tick int NOT NULL DEFAULT 0;
ALTER TABLE events ALTER COLUMN world_tick SET DEFAULT current_world_tick();

CREATE INDEX idx_events_stream_tick ON events (stream_id, world_tick);
