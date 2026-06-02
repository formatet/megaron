-- Migration 028: terrain enum expansion
-- Renames legacy terrain strings to the full Bronze Age vocabulary.
-- New mapgen will also generate 'coastal_sea' (sea tiles adjacent to land);
-- existing worlds get 'deep_sea' for all former 'sea' tiles.

UPDATE map_tiles SET terrain = 'coast_beach'        WHERE terrain = 'coast';
UPDATE map_tiles SET terrain = 'forest_olive_grove' WHERE terrain = 'forest';
UPDATE map_tiles SET terrain = 'mountain_limestone' WHERE terrain = 'mountain';
UPDATE map_tiles SET terrain = 'deep_sea'           WHERE terrain = 'sea';

UPDATE provinces SET terrain_type = 'coast_beach'        WHERE terrain_type = 'coast';
UPDATE provinces SET terrain_type = 'forest_olive_grove' WHERE terrain_type = 'forest';
UPDATE provinces SET terrain_type = 'mountain_limestone' WHERE terrain_type = 'mountain';
UPDATE provinces SET terrain_type = 'deep_sea'           WHERE terrain_type = 'sea';

UPDATE production_rules SET terrain_type = 'forest_olive_grove' WHERE terrain_type = 'forest';
UPDATE production_rules SET terrain_type = 'mountain_limestone' WHERE terrain_type = 'mountain';
UPDATE production_rules SET terrain_type = 'coast_beach'        WHERE terrain_type = 'coast';
