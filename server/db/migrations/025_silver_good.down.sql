DELETE FROM settlement_goods WHERE good_key = 'silver';
DELETE FROM production_rules WHERE good_key = 'silver';
ALTER TABLE provinces DROP COLUMN IF EXISTS silver_deposit;
ALTER TABLE map_tiles  DROP COLUMN IF EXISTS silver_deposit;
DELETE FROM goods WHERE key = 'silver';
