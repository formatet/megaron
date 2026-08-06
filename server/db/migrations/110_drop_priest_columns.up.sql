-- Priest was retired as a unit in mig 060; what remained were three physical
-- columns the code no longer references at all — verified 2026-08-06: no Go
-- reads or writes to any of them after the priest code sweep, and 0 rows
-- populated (marching_armies.priest / borrowed_armies.priest are always 0,
-- temples.priest_id always NULL). Drop them so no schema trace of priest is
-- left (Timothy 2026-08-06). marching_armies/borrowed_armies still carry the
-- other flat army columns (infantry, chariot, ship, ...) — only priest goes.
ALTER TABLE marching_armies DROP COLUMN priest;
ALTER TABLE borrowed_armies DROP COLUMN priest;
ALTER TABLE temples DROP COLUMN priest_id;
