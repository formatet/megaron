-- Revert 121: remove the terrain-free refining rows (olive_press/winery/foundry).
DELETE FROM production_rules
 WHERE terrain_type IS NULL
   AND building_type IN ('olive_press', 'winery', 'foundry')
   AND good_key IN ('oil', 'wine', 'bronze');
