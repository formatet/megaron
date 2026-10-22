-- ORDER 15 (Timothy 2026-10-22): every temple is dedicated to one god; a city
-- may hold one temple per god. Temples standing or queued before this belong to
-- the start god, Potnia (decision 2). Temple workers stand at one temple.
ALTER TABLE buildings ADD COLUMN god_id text;
ALTER TABLE build_queue ADD COLUMN god_id text;
ALTER TABLE settlement_placement ADD COLUMN building_id uuid REFERENCES buildings(id) ON DELETE CASCADE;

UPDATE buildings SET god_id = 'potnia' WHERE building_type = 'temple';
UPDATE build_queue SET god_id = 'potnia' WHERE building_type = 'temple';
UPDATE settlement_placement p SET building_id = b.id
  FROM buildings b
 WHERE p.target_kind = 'building' AND p.building_type = 'temple'
   AND b.settlement_id = p.settlement_id AND b.building_type = 'temple';

ALTER TABLE buildings ADD CONSTRAINT buildings_temple_god_check
  CHECK ((building_type = 'temple') = (god_id IS NOT NULL));
ALTER TABLE build_queue ADD CONSTRAINT build_queue_temple_god_check
  CHECK ((building_type = 'temple') = (god_id IS NOT NULL));
ALTER TABLE settlement_placement ADD CONSTRAINT settlement_placement_temple_building_check
  CHECK ((building_type IS NOT DISTINCT FROM 'temple') = (building_id IS NOT NULL));

DROP INDEX buildings_settlement_building_city;
CREATE UNIQUE INDEX buildings_settlement_building_city
    ON buildings (settlement_id, building_type) WHERE hex_q IS NULL AND building_type <> 'temple';
CREATE UNIQUE INDEX buildings_settlement_temple_god
    ON buildings (settlement_id, god_id) WHERE building_type = 'temple';
