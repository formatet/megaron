-- The tick a settlement was sacked and burned (combat/occupation.go burn
-- branch). The map draws the fire for exactly that one tick — one game day —
-- and the ruin afterwards. NULL for every settlement that was never burned;
-- rows razed before this migration stay NULL and never burn on the map.
ALTER TABLE settlements ADD COLUMN burned_tick int;
