-- Återställ messengers till bosättning-destination only.
DELETE FROM messengers WHERE kind = 'recall';
ALTER TABLE messengers ALTER COLUMN destination_id SET NOT NULL;
ALTER TABLE messengers
    DROP COLUMN kind,
    DROP COLUMN dest_q,
    DROP COLUMN dest_r;
