-- Retire queued loyalty work, including previously failed rows, without a
-- dead-letter or a replacement job. Audit events remain in the append-only log.
UPDATE scheduled_events
SET processed_at = now(), failed_at = NULL
WHERE event_type IN ('LoyaltyDecayTick', 'LoyaltyWelfareTick', 'ColonyPenaltyTick', 'BorrowedArmyTick')
  AND processed_at IS NULL;

DROP TABLE loyalty_events;
ALTER TABLE settlements
  DROP COLUMN loyalty,
  DROP COLUMN loyalty_points,
  DROP COLUMN loyalty_trend;
