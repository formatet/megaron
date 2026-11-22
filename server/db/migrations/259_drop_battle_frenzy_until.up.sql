-- battle_frenzy_until (mig 020) has no reader or writer in Go, SQL, tools or web
-- (verified 2026-11-22). Dropped; the down migration restores the empty column.
ALTER TABLE settlements DROP COLUMN IF EXISTS battle_frenzy_until;
