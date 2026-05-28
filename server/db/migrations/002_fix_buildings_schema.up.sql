-- buildings.type → building_type (all code uses building_type)
ALTER TABLE buildings RENAME COLUMN type TO building_type;

-- ON CONFLICT (province_id, building_type) requires a unique constraint
ALTER TABLE buildings ADD CONSTRAINT buildings_province_building UNIQUE (province_id, building_type);
