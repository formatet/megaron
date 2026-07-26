DROP TABLE IF EXISTS unit_ordinals;
DROP INDEX IF EXISTS idx_units_support_settlement;
ALTER TABLE units DROP COLUMN IF EXISTS ordinal;
ALTER TABLE units DROP COLUMN IF EXISTS support_settlement_id;
