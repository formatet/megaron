DROP FUNCTION IF EXISTS terrain_is_thalassa(text);
DELETE FROM production_rules WHERE terrain_type = 'lake';
UPDATE provinces SET terrain_type = 'coastal_sea' WHERE terrain_type = 'lake';
UPDATE map_tiles SET terrain = 'coastal_sea' WHERE terrain = 'lake';
CREATE OR REPLACE FUNCTION terrain_is_sea(t text) RETURNS boolean
LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$ SELECT t IN ('coastal_sea','deep_sea') $$;
CREATE OR REPLACE FUNCTION terrain_is_navigable(t text) RETURNS boolean
LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$ SELECT t IN ('coastal_sea','deep_sea','river','river_ford') $$;
