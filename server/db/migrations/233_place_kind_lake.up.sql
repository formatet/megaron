-- Lakes are nameable places (insjöar slice 3, Timothy 2026-10-30): every size,
-- naming right to the first ship that sails onto one.
ALTER TABLE place_names DROP CONSTRAINT place_names_kind_check;
ALTER TABLE place_names ADD CONSTRAINT place_names_kind_check CHECK (kind IN ('river','massif','desert','forest','lake'));
