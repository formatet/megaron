-- Migration 053: W4b — recept-ramverk. Andra craft-kedjan utöver bronze:
-- luxury = 2 purple + 3 oil → 1 luxury i market (urban verkstad/handelshub).
-- Bevisar att recipes/recipe_ingredients driver materialträdet generiskt —
-- ProvinceHandler.Craft slår upp recept per id utan hårdkodad bronze-gren.

INSERT INTO recipes (output_key, output_qty, building_type, duration_min)
    VALUES ('luxury', 1.0, 'market', 60);

INSERT INTO recipe_ingredients (recipe_id, good_key, quantity)
    SELECT r.id, u.good_key, u.qty
    FROM recipes r
    CROSS JOIN (VALUES ('purple', 2.0::float), ('oil', 3.0::float)) AS u(good_key, qty)
    WHERE r.output_key = 'luxury' AND r.building_type = 'market';
