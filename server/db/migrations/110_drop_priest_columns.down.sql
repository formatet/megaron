-- Reverse of 110: restore the three retired priest columns exactly as they
-- stood before the drop. Values are not recoverable, but they were always 0 /
-- NULL, so the original defaults reproduce the pre-drop state in full.
--   marching_armies.priest  integer NOT NULL DEFAULT 0
--   borrowed_armies.priest  integer NOT NULL DEFAULT 0
--   temples.priest_id       uuid, nullable, FK -> players(id)
ALTER TABLE marching_armies ADD COLUMN priest integer NOT NULL DEFAULT 0;
ALTER TABLE borrowed_armies ADD COLUMN priest integer NOT NULL DEFAULT 0;
ALTER TABLE temples ADD COLUMN priest_id uuid REFERENCES players(id);
