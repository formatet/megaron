-- Prayers of speed (bönesystemet v3 steg 2b). A speed blessing can name a Runner or a
-- land caravan, which are not units: mover_id/mover_kind name it (no FK: messengers or transports).
DO $$
DECLARE c text;
BEGIN
  FOR c IN SELECT conname FROM pg_constraint
            WHERE conrelid = 'prayer_blessings'::regclass AND contype = 'c' AND pg_get_constraintdef(oid) LIKE '%unit_id%'
  LOOP
    EXECUTE format('ALTER TABLE prayer_blessings DROP CONSTRAINT %I', c);
  END LOOP;
END $$;
ALTER TABLE prayer_blessings ADD COLUMN mover_id uuid, ADD COLUMN mover_kind text CHECK (mover_kind IN ('messenger','transport'));
ALTER TABLE prayer_blessings
  ADD CONSTRAINT prayer_blessings_unit_target CHECK ((scope = 'unit') = (unit_id IS NOT NULL OR mover_id IS NOT NULL)),
  ADD CONSTRAINT prayer_blessings_one_target CHECK (unit_id IS NULL OR mover_id IS NULL),
  ADD CONSTRAINT prayer_blessings_mover_kind CHECK ((mover_id IS NULL) = (mover_kind IS NULL));
