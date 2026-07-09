-- Ship-build overhaul (Timothy 2026-07-09, temenos_enheter.md "Flottdesign"):
-- ships are built one vessel at a time and take build time before they are
-- deployable, and they get a name (Wanax-chosen, or game-suggested from a
-- culture-appropriate pool).
ALTER TABLE units ADD COLUMN IF NOT EXISTS name TEXT;
ALTER TABLE units ADD COLUMN IF NOT EXISTS build_complete_at TIMESTAMPTZ;
