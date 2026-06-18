-- Revert migration 052: ta bort purple/pottery/luxury och deras produktionsregler.
DELETE FROM production_rules WHERE good_key IN ('purple', 'pottery');
DELETE FROM settlement_goods WHERE good_key IN ('purple', 'pottery', 'luxury');
DELETE FROM goods WHERE key IN ('purple', 'pottery', 'luxury');
