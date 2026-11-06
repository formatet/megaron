ALTER TABLE prayer_blessings DROP CONSTRAINT IF EXISTS prayer_blessings_unit_target, DROP CONSTRAINT IF EXISTS prayer_blessings_one_target, DROP CONSTRAINT IF EXISTS prayer_blessings_mover_kind;
DELETE FROM prayer_blessings WHERE mover_id IS NOT NULL;
ALTER TABLE prayer_blessings DROP COLUMN IF EXISTS mover_id, DROP COLUMN IF EXISTS mover_kind;
ALTER TABLE prayer_blessings ADD CONSTRAINT prayer_blessings_unit_scope CHECK ((scope = 'unit') = (unit_id IS NOT NULL));
