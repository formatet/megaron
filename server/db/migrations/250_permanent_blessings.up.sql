-- Prayers of permanence (bönesystemet v3 steg 4, Timothy 2026-11-07): a blessing laid on one
-- NAMED unit lasts for ever.
--   prayer_blessings.permanent : strength / speed. until_tick is then int4-max, so every reader
--                                that tests until_tick > tick (LoadBlessed, BlessingActive,
--                                ConsumeWard, SpeedBlessingFor) stays correct without a special case.
--   units.hull_bonus           : hull (Stout Hulls): +1 max hull for ever. The ship's ceiling is
--                                combat.HullMax + hull_bonus, per ship.
ALTER TABLE prayer_blessings ADD COLUMN permanent boolean NOT NULL DEFAULT false;
ALTER TABLE units ADD COLUMN hull_bonus integer NOT NULL DEFAULT 0 CHECK (hull_bonus BETWEEN 0 AND 1);
ALTER TABLE units DROP CONSTRAINT units_hull_check;
ALTER TABLE units ADD CONSTRAINT units_hull_within_cap CHECK (hull >= 0 AND hull <= 5 + hull_bonus);
