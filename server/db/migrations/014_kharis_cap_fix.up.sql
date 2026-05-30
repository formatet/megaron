ALTER TABLE settlements ALTER COLUMN kharis_cap SET DEFAULT 2000;
UPDATE settlements SET kharis_cap = 2000 WHERE kharis_cap <= 100;
