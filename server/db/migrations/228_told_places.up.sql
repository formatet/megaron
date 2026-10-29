-- Telling of places (megaron_plan_vagbeskrivningar v1, Timothy 2026-10-28
-- 20:50/20:53): a letter may tell of a named place. The story is frozen on
-- the messenger when it leaves and handed over on delivery; told hexes land
-- in player_heard_tiles (source 'told') with the teller beside them.
ALTER TABLE messengers ADD COLUMN told_place JSONB;
ALTER TABLE player_heard_tiles ADD COLUMN told_by UUID REFERENCES players(id) ON DELETE SET NULL;
