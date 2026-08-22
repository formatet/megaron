-- Migration 127: dedup for self-rescheduling tick events (G2 tail)
-- (megaron_plan_tickdedup.md, Timothy 2026-08-22)
--
-- The self-rescheduling handlers (colony.go, kharis/tick.go, loyalty/decay.go,
-- upkeep, …) call EnqueueTickRecurring as the LAST line of their handler
-- body. If events.Worker retries a handler — which it does on timeout/error,
-- up to DeadLetterAttempts times — that line runs twice and two rows land in
-- scheduled_events for the same next tick. Each subsequent firing doubles
-- again (2 → 4 → 8), silently corrupting the tick cadence without any
-- visible error.
--
-- payload MUST be part of the index: BattleTick and OccupationCheck
-- legitimately have several concurrent pending rows sharing
-- (world_id, event_type, due_tick) that differ only in payload (one row per
-- active battle / occupied settlement). An index without payload would
-- silently merge two unrelated battles — worse than the bug being fixed
-- here. payload is JSONB (has a btree opclass), so it compares fine in a
-- unique index.
--
-- Existing duplicates found in poleia_test_dedup (a template clone) before
-- this migration: one group, 2 rows — a MarchSightingScan for the same
-- world/due_tick/empty-payload. Deduped below (keep lowest id) before the
-- index is created, since a duplicate already in the table would otherwise
-- make CREATE UNIQUE INDEX fail.
DELETE FROM scheduled_events se
USING scheduled_events se2
WHERE se.processed_at IS NULL
  AND se2.processed_at IS NULL
  AND se.world_id = se2.world_id
  AND se.event_type = se2.event_type
  AND se.due_tick = se2.due_tick
  AND se.payload = se2.payload
  AND se.id > se2.id;

CREATE UNIQUE INDEX idx_scheduled_recurring_dedup
    ON scheduled_events (world_id, event_type, due_tick, payload)
    WHERE processed_at IS NULL;
