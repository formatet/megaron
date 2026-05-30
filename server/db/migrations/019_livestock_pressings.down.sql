-- Undo migration 019

DELETE FROM production_rules WHERE building_type IN ('olive_press', 'winery')
    OR (terrain_type = 'plains' AND good_key = 'livestock' AND building_type IS NULL);

DELETE FROM settlement_goods WHERE good_key = 'livestock';

DELETE FROM goods WHERE key = 'livestock';
