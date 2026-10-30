-- R2 battle, mild (Timothy 2026-10-30): the gods' hand a unit carries into a
-- battle, fixed when it joins (religion.BattleDivine). NULL: none.
ALTER TABLE battle_participants ADD COLUMN divine text CHECK (divine IN ('favor','wrath'));
