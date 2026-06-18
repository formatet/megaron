-- Revert migration 053: ta bort luxury-receptet.
DELETE FROM recipe_ingredients
    WHERE recipe_id IN (SELECT id FROM recipes WHERE output_key = 'luxury' AND building_type = 'market');
DELETE FROM recipes WHERE output_key = 'luxury' AND building_type = 'market';
