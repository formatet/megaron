-- Migration 099 down: revert bronze recipe copper 9 -> 2 (tin unchanged at 1)

UPDATE recipe_ingredients
SET quantity = 2.0
WHERE recipe_id = (
    SELECT id FROM recipes WHERE output_key = 'bronze' AND building_type = 'foundry'
)
AND good_key = 'copper';
