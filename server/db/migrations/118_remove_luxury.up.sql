-- Migration 118: ta bort luxury — HELT, inte parkera (Timothy 2026-08-07,
-- megaron_plan_varukatalogen.md S4). mig 115 parkerade purple/pottery/horses
-- men lämnade luxury uttryckligen åt detta separata beslut: full avveckling,
-- ingen konvertering till annan vara (det vore att mynta värde ur luft,
-- taxonomins §1.4).
--
-- Ordning: receptet (mig 053, cascadar recipe_ingredients) → varje annan
-- good_key-FK-tabell → goods-raden själv (FK-mål för alla ovan).
DELETE FROM recipes WHERE output_key = 'luxury';
DELETE FROM settlement_goods WHERE good_key = 'luxury';
DELETE FROM settlement_labor WHERE good_key = 'luxury';
DELETE FROM transport_goods WHERE good_key = 'luxury';
DELETE FROM settlement_granary WHERE good_key = 'luxury';
DELETE FROM divine_valuations WHERE good_key = 'luxury';
DELETE FROM production_rules WHERE good_key = 'luxury';
DELETE FROM goods WHERE key = 'luxury';
