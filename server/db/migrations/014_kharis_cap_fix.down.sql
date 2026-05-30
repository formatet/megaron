ALTER TABLE settlements ALTER COLUMN kharis_cap SET DEFAULT 100;
UPDATE settlements SET kharis_cap = 100;
