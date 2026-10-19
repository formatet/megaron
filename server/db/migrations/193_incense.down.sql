DELETE FROM settlement_placement WHERE good_key = 'incense';
DELETE FROM settlement_labor WHERE good_key = 'incense';
DELETE FROM production_rules WHERE good_key = 'incense';
DELETE FROM settlement_goods WHERE good_key = 'incense';
DELETE FROM divine_labor_valuations WHERE good_key = 'incense';
DELETE FROM divine_valuations WHERE good_key = 'incense';
DELETE FROM goods WHERE key = 'incense';
ALTER TABLE provinces DROP COLUMN IF EXISTS incense_deposit;
ALTER TABLE map_tiles DROP COLUMN IF EXISTS incense_deposit;
