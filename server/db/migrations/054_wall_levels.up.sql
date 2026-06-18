-- Migration 054: W4c — mur som EN nivåbyggnad. Collapse:a wall/tower/bronze_wall
-- till en enda wall-rad med level = settlements.wall_level. wall_level (på
-- settlements) är oförändrad — striden läser den direkt.
DELETE FROM buildings WHERE building_type IN ('wall', 'tower', 'bronze_wall');

INSERT INTO buildings (settlement_id, building_type, level)
SELECT id, 'wall', LEAST(GREATEST(wall_level, 1), 3)
FROM settlements
WHERE wall_level >= 1
ON CONFLICT (settlement_id, building_type) DO UPDATE SET level = EXCLUDED.level;
