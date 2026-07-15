-- Återställ messengers till settlement-origin only.
DELETE FROM messengers WHERE origin_unit_id IS NOT NULL;
ALTER TABLE messengers DROP CONSTRAINT messengers_unit_origin_has_coords;
ALTER TABLE messengers DROP CONSTRAINT messengers_exactly_one_origin;
ALTER TABLE messengers ALTER COLUMN origin_id SET NOT NULL;
ALTER TABLE messengers
    DROP COLUMN origin_unit_id,
    DROP COLUMN origin_q,
    DROP COLUMN origin_r;
