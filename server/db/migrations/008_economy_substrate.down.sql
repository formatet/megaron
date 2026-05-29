-- Migration 008 rollback
DROP TABLE IF EXISTS production_rules;
DROP TABLE IF EXISTS settlement_goods;
ALTER TABLE provinces  DROP COLUMN IF EXISTS copper_deposit;
ALTER TABLE provinces  DROP COLUMN IF EXISTS tin_deposit;
ALTER TABLE map_tiles  DROP COLUMN IF EXISTS copper_deposit;
ALTER TABLE map_tiles  DROP COLUMN IF EXISTS tin_deposit;
DROP TABLE IF EXISTS goods;
DROP INDEX  IF EXISTS buildings_settlement_building;
