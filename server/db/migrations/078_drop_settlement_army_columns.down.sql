-- Re-add the frozen army columns (zeroed). Note: this cannot recover the pre-drop
-- values — the army has lived in the units table since mig 047, so the true army
-- is unaffected either way.
ALTER TABLE settlements
    ADD COLUMN IF NOT EXISTS infantry       INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS chariot        INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS priest         INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS ship           INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS elite_infantry INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS war_galley     INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS merchantman    INTEGER NOT NULL DEFAULT 0;
