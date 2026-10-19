DROP FUNCTION credit_settlement_good(UUID,TEXT,DOUBLE PRECISION,TEXT,DOUBLE PRECISION);
DROP FUNCTION storage_capped(UUID,TEXT,DOUBLE PRECISION,DOUBLE PRECISION,TEXT);
-- Ceilings cannot be lowered safely; existing balances are deliberately retained.
