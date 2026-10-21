DROP TRIGGER buildings_silo_cap ON buildings;
DROP TRIGGER settlement_goods_silo_cap ON settlement_goods;
DROP FUNCTION buildings_silo_cap();
DROP FUNCTION settlement_goods_silo_cap();
DROP FUNCTION settlement_silo_level(UUID);
DROP FUNCTION silo_stored_good(TEXT);
DROP FUNCTION silo_food_mult(INT);
