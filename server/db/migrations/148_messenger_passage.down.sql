DROP INDEX IF EXISTS idx_messengers_passage_aboard;
DROP INDEX IF EXISTS idx_messengers_passage_waiting;

ALTER TABLE messengers
    DROP COLUMN IF EXISTS carrier_name,
    DROP COLUMN IF EXISTS carrier_unit_id,
    DROP COLUMN IF EXISTS carrier_transport_id,
    DROP COLUMN IF EXISTS passage_lost_until_tick,
    DROP COLUMN IF EXISTS passage_since_tick,
    DROP COLUMN IF EXISTS passage_port_id,
    DROP COLUMN IF EXISTS passage_status;
