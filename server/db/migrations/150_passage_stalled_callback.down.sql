ALTER TABLE messengers
    DROP COLUMN IF EXISTS withdrawn,
    DROP COLUMN IF EXISTS passage_stalled_notified_tick;
