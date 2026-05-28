ALTER TABLE buildings DROP CONSTRAINT IF EXISTS buildings_province_building;
ALTER TABLE buildings RENAME COLUMN building_type TO type;
