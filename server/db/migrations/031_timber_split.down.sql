-- Revert migration 031: timber / ädelträ split
DELETE FROM settlement_goods WHERE good_key = 'timber';
DELETE FROM production_rules WHERE good_key = 'timber';
DELETE FROM goods WHERE key = 'timber';
