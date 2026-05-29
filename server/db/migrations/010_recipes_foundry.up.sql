-- Migration 010: Craft recipes and foundry building type

CREATE TABLE recipes (
    id            SERIAL PRIMARY KEY,
    output_key    TEXT NOT NULL REFERENCES goods(key),
    output_qty    FLOAT NOT NULL,
    building_type TEXT NOT NULL,  -- building required to craft
    duration_min  FLOAT NOT NULL  -- reserved for future craft queues
);

CREATE TABLE recipe_ingredients (
    id        SERIAL PRIMARY KEY,
    recipe_id INTEGER NOT NULL REFERENCES recipes(id) ON DELETE CASCADE,
    good_key  TEXT NOT NULL REFERENCES goods(key),
    quantity  FLOAT NOT NULL
);

-- Bronze: 2 copper + 1 tin → 1 bronze (foundry, 60 min)
INSERT INTO recipes (output_key, output_qty, building_type, duration_min)
    VALUES ('bronze', 1.0, 'foundry', 60);

INSERT INTO recipe_ingredients (recipe_id, good_key, quantity)
    SELECT r.id, u.good_key, u.qty
    FROM recipes r
    CROSS JOIN (VALUES ('copper', 2.0::float), ('tin', 1.0::float)) AS u(good_key, qty)
    WHERE r.output_key = 'bronze' AND r.building_type = 'foundry';
