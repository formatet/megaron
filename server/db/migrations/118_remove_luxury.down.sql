-- Återställer katalogen (mig 052+053+093): luxury-varan och dess recept.
-- Instansdata (settlement_goods/settlement_labor/transport_goods m.fl.)
-- återskapas INTE — den var borttagen med avsikt och kan inte härledas.
INSERT INTO goods (key, name, tier, category, base_value, weight, religious) VALUES
    ('luxury', 'Luxury', 'manufactured', 'prestige', 30.0, 1.0, true)
ON CONFLICT (key) DO NOTHING;

INSERT INTO recipes (output_key, output_qty, building_type)
    VALUES ('luxury', 1.0, 'market');

INSERT INTO recipe_ingredients (recipe_id, good_key, quantity)
    SELECT r.id, u.good_key, u.qty
    FROM recipes r
    CROSS JOIN (VALUES ('purple', 2.0::float), ('oil', 3.0::float)) AS u(good_key, qty)
    WHERE r.output_key = 'luxury' AND r.building_type = 'market';
