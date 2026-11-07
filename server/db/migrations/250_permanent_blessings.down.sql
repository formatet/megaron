DELETE FROM prayer_blessings WHERE permanent;
UPDATE units SET hull = LEAST(hull, 5);
ALTER TABLE units DROP CONSTRAINT units_hull_within_cap;
ALTER TABLE units DROP COLUMN hull_bonus;
ALTER TABLE units ADD CONSTRAINT units_hull_check CHECK (hull BETWEEN 0 AND 5);
ALTER TABLE prayer_blessings DROP COLUMN permanent;
