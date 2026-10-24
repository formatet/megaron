-- Refuse a lossy rollback while a hull has multiple passengers.
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM units WHERE carried_by_ship_id IS NOT NULL GROUP BY carried_by_ship_id HAVING count(*)>1) THEN
  RAISE EXCEPTION 'cannot roll back ship passengers while a ship carries multiple units';
 END IF;
END $$;
ALTER TABLE units ADD COLUMN cargo_unit_id uuid REFERENCES units(id) ON DELETE SET NULL;
UPDATE units ship SET cargo_unit_id=passenger.id FROM units passenger WHERE passenger.carried_by_ship_id=ship.id;
DROP INDEX units_carried_by_ship_idx;
ALTER TABLE units DROP COLUMN carried_by_ship_id;
