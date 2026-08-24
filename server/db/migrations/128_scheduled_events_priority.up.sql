-- Körordning inom ett tick (megaron_tickordning.md).
--
-- Before this column, every event due at the same tick was claimed by
-- processBatch's `ORDER BY due_tick, id` and dispatched in whatever order
-- RETURNING happened to hand back — Postgres guarantees no ordering for
-- RETURNING, so a subquery's ORDER BY only picks WHICH rows are claimed, never
-- the order they come back in. Measured 48.2/51.8 % over 438 ticks: a coin
-- flip, every tick, between every pair of same-tick handlers.
--
-- That coin flip was not cosmetic. KharisTick's grain-funded growth draws the
-- settlement's grain stock down to (near) zero by design, so on the ticks it
-- won the flip, UpkeepTick found nothing left and the garrison took
-- grain_shortage attrition in a city with a healthy surplus. Reproduced in the
-- acceptance world 2026-08-24 at tick 1, in BOTH capitals.
--
-- priority makes the order explicit and sortable in SQL, so the ordering also
-- holds ACROSS batches (LIMIT 20 means a busy world's tick spans many batches).
-- The authority for the values is events.TickPriority in Go; this column is
-- written at enqueue time. 50 is the neutral default that rows enqueued before
-- this migration keep — they simply all compare equal, exactly as today.
ALTER TABLE scheduled_events
    ADD COLUMN priority SMALLINT NOT NULL DEFAULT 50;

-- The claim query filters on (processed_at, failed_at, world_id, due_tick) and
-- now sorts on (due_tick, priority, id). Extend the ordering index accordingly.
CREATE INDEX IF NOT EXISTS idx_scheduled_events_due_priority
    ON scheduled_events (due_tick, priority, id)
    WHERE processed_at IS NULL AND failed_at IS NULL;
