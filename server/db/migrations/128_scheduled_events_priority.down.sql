DROP INDEX IF EXISTS idx_scheduled_events_due_priority;
ALTER TABLE scheduled_events DROP COLUMN IF EXISTS priority;
