-- The passenger owns its one physical carrier. Preserve existing single cargo.
ALTER TABLE units ADD COLUMN carried_by_ship_id uuid REFERENCES units(id) ON DELETE SET NULL;
CREATE INDEX units_carried_by_ship_idx ON units(carried_by_ship_id) WHERE carried_by_ship_id IS NOT NULL;
UPDATE units passenger SET carried_by_ship_id=ship.id
FROM units ship WHERE ship.cargo_unit_id=passenger.id;
ALTER TABLE units DROP COLUMN cargo_unit_id;
