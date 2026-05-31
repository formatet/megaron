-- Priests are no longer combat units; priest_strength is replaced by
-- the kharis-based rite system. battle_frenzy_until tracks active frenzy buffs.
ALTER TABLE settlements DROP COLUMN IF EXISTS priest_strength;
ALTER TABLE settlements ADD COLUMN IF NOT EXISTS battle_frenzy_until TIMESTAMPTZ;
