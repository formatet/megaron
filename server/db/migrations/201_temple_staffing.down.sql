-- Converted placements are physical jobs. Never silently invent old shares or
-- restore additive workers on rollback. Restore a pre-transition backup instead.
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM temple_staffing_transition WHERE completed) THEN
  RAISE EXCEPTION 'temple staffing converted: restore pre-transition backup for rollback';
 END IF;
END $$;
DROP TABLE temple_staffing_transition;
