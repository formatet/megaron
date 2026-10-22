-- Refuses when a city holds more than one temple: collapsing them would lose a building.
DROP INDEX buildings_settlement_temple_god;
DROP INDEX buildings_settlement_building_city;
CREATE UNIQUE INDEX buildings_settlement_building_city
    ON buildings (settlement_id, building_type) WHERE hex_q IS NULL;
ALTER TABLE settlement_placement DROP CONSTRAINT settlement_placement_temple_building_check;
ALTER TABLE build_queue DROP CONSTRAINT build_queue_temple_god_check;
ALTER TABLE buildings DROP CONSTRAINT buildings_temple_god_check;
ALTER TABLE settlement_placement DROP COLUMN building_id;
ALTER TABLE build_queue DROP COLUMN god_id;
ALTER TABLE buildings DROP COLUMN god_id;
