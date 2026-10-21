-- A runner may board any Wanax's ship (Timothy 2026-10-21, temenos_diplomati §Runners).
-- The lost-carrier check compares the ship's owner with the owner at boarding,
-- no longer with the runner's sender.
ALTER TABLE messengers ADD COLUMN carrier_owner_id UUID REFERENCES players(id) ON DELETE SET NULL;
