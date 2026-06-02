-- Reverse migration 028
UPDATE map_tiles SET terrain = 'coast'    WHERE terrain = 'coast_beach';
UPDATE map_tiles SET terrain = 'forest'   WHERE terrain = 'forest_olive_grove';
UPDATE map_tiles SET terrain = 'mountain' WHERE terrain = 'mountain_limestone';
UPDATE map_tiles SET terrain = 'sea'      WHERE terrain IN ('deep_sea', 'coastal_sea');

UPDATE provinces SET terrain_type = 'coast'    WHERE terrain_type = 'coast_beach';
UPDATE provinces SET terrain_type = 'forest'   WHERE terrain_type = 'forest_olive_grove';
UPDATE provinces SET terrain_type = 'mountain' WHERE terrain_type = 'mountain_limestone';
UPDATE provinces SET terrain_type = 'sea'      WHERE terrain_type IN ('deep_sea', 'coastal_sea');

UPDATE production_rules SET terrain_type = 'forest'   WHERE terrain_type = 'forest_olive_grove';
UPDATE production_rules SET terrain_type = 'mountain' WHERE terrain_type = 'mountain_limestone';
UPDATE production_rules SET terrain_type = 'coast'    WHERE terrain_type = 'coast_beach';
